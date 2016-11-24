package main

import (
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
)

const (
	tokenPath = "/var/run/secrets/boostport.com/vault-token"
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

	w.Flush()

	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	<-sigs
}
