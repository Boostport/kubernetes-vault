package main

import (
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"syscall"
	"text/tabwriter"
)

const (
	credentialsPath = "/var/run/secrets/boostport.com"
)

type authToken struct {
	ClientToken   string `json:"clientToken"`
	Accessor      string `json:"accessor"`
	LeaseDuration int    `json:"leaseDuration"`
	Renewable     bool   `json:"renewable"`
	VaultAddr     string `json:"vaultAddr"`
}

func main() {

	logger := logrus.New()
	logger.Level = logrus.DebugLevel

	tokenPath := path.Join(credentialsPath, "vault-token")
	content, err := ioutil.ReadFile(tokenPath)

	if err != nil {
		logger.Fatalf("Error opening token file (%s): %s", tokenPath, err)
	}

	var token authToken

	err = json.Unmarshal(content, &token)

	if err != nil {
		logger.Fatalf("Error unmarhsaling JSON: %s", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Token:\t%s\n", token.ClientToken)
	fmt.Fprintf(w, "Accessor:\t%s\n", token.Accessor)
	fmt.Fprintf(w, "Lease Duration:\t%d\n", token.LeaseDuration)
	fmt.Fprintf(w, "Renewable:\t%t\n", token.Renewable)
	fmt.Fprintf(w, "Vault Address:\t%s\n", token.VaultAddr)

	caBundlePath := path.Join(credentialsPath, "ca.crt")

	_, err = os.Stat(caBundlePath)

	caBundleExists := true

	if err != nil && os.IsNotExist(err) {
		caBundleExists = false
	}

	fmt.Fprintf(w, "CA Bundle Exists:\t%t\n", caBundleExists)

	w.Flush()

	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	<-sigs
}
