package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/Boostport/kubernetes-vault/common"
	"github.com/Sirupsen/logrus"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
)

type authToken struct {
	ClientToken   string `json:"clientToken"`
	Accessor      string `json:"accessor"`
	LeaseDuration int    `json:"leaseDuration"`
	Renewable     bool   `json:"renewable"`
	VaultAddr     string `json:"vaultAddr"`
}

func main() {

	logLevel := os.Getenv("LOG_LEVEL")

	logger := logrus.New()
	logger.Level = logrus.DebugLevel

	if logLevel != "debug" && logLevel != "error" {
		logger.Fatalf(`Invalid LOG_LEVEL. Valid values are "debug" and "error".`)
	}

	if logLevel == "error" {
		logger.Level = logrus.ErrorLevel
	}

	roleId := os.Getenv("VAULT_ROLE_ID")

	if roleId == "" {
		logger.Fatal("The VAULT_ROLE_ID environment variable must be set.")
	}

	timeoutStr := os.Getenv("TIMEOUT")

	var (
		timeout time.Duration
		err     error
	)

	if timeoutStr == "" {
		timeout = 5 * time.Minute
	} else {

		timeout, err = time.ParseDuration(timeoutStr)

		if err != nil {
			logger.Fatalf("Invalid timeout (%s): %s", timeoutStr, err)
		}
	}

	credentialsPath := os.Getenv("CREDENTIALS_PATH")

	if credentialsPath == "" {
		credentialsPath = "/var/run/secrets/boostport.com"
	}

	ip, err := common.ExternalIP()

	if err != nil {
		logger.Fatalf("Error looking up external ip for container: %s", err)
	}

	certificate, err := generateCertificate(ip, timeout)

	if err != nil {
		logger.Fatalf("Could not generate self-signed certificate: %s", err)
	}

	result := make(chan common.WrappedSecretId)

	go startHTTPServer(certificate, logger, result)

	for {
		select {
		case wrappedSecretId := <-result:

			authToken, rootCAs, err := processWrappedSecretId(wrappedSecretId, roleId)

			if err != nil {
				logger.Fatalf("Could not get auth token: %s", err)
			}

			b, err := json.Marshal(authToken)

			if err != nil {
				logger.Fatalf("Could not marshal auth token to JSON: %s", err)
			}

			tokenPath := path.Join(credentialsPath, "vault-token")

			err = ioutil.WriteFile(tokenPath, b, 0444)

			if err != nil {
				logger.Fatalf("Could not write auth token to path (%s): %s", tokenPath, err)
			}

			if len(rootCAs) > 0 {

				caBundlePath := path.Join(credentialsPath, "ca.crt")

				err = ioutil.WriteFile(caBundlePath, rootCAs, 0444)

				if err != nil {
					logger.Fatalf("Could not write CA bundle to path (%s): %s", caBundlePath, err)
				}
			}

			logger.Debug("Successfully created the vault token. Exiting.")
			os.Exit(0)

		case <-time.After(timeout):
			logger.Fatalf("Failed to create vault auth token because we timed out after %s before receiving the secret_id. Exiting.", timeout)
		}
	}

}

func startHTTPServer(certificate tls.Certificate, logger *logrus.Logger, wrappedSecretId chan<- common.WrappedSecretId) {
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
	}

	tlsConfig.BuildNameToCertificate()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {

		if req.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if req.Method == "POST" {

			decoder := json.NewDecoder(req.Body)

			var wrappedSecret common.WrappedSecretId

			err := decoder.Decode(&wrappedSecret)

			if err != nil {
				logger.Debugf("Error decoding wrapped secret: %s", err)
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Could not decode wrapped secret."))
				return
			}

			wrappedSecretId <- wrappedSecret
			w.WriteHeader(http.StatusOK)
			return

		} else {
			logger.Debugf("The / endpoint only support POSTs")
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("The / endpoint only support POSTs"))
		}

	})

	server := &http.Server{
		Handler:   mux,
		Addr:      fmt.Sprintf(":%d", common.InitContainerPort),
		TLSConfig: tlsConfig,
	}

	server.ListenAndServeTLS("", "")
}

func processWrappedSecretId(wrappedSecretId common.WrappedSecretId, roleId string) (authToken, []byte, error) {

	rootCAs := []byte{}

	if err := wrappedSecretId.Validate(); err != nil {
		return authToken{}, rootCAs, errors.Wrap(err, "could not validate wrapped secret_id")
	}

	var roots *x509.CertPool

	if len(wrappedSecretId.VaultCAs) > 0 {
		roots = x509.NewCertPool()

		roots.AppendCertsFromPEM([]byte(wrappedSecretId.VaultCAs))
		rootCAs = wrappedSecretId.VaultCAs
	}

	httpClient := cleanhttp.DefaultPooledClient()

	if roots != nil {
		tlsConfig := &tls.Config{RootCAs: roots}
		httpClient.Transport.(*http.Transport).TLSClientConfig = tlsConfig
	}

	client, err := api.NewClient(&api.Config{Address: wrappedSecretId.VaultAddr, HttpClient: httpClient})

	client.SetToken(wrappedSecretId.Token)

	if err != nil {
		return authToken{}, rootCAs, errors.Wrap(err, "could not create vault client")
	}

	secret, err := client.Logical().Unwrap("")

	if err != nil {
		return authToken{}, rootCAs, errors.Wrap(err, "error unwrapping secret_id")
	}

	secretId, ok := secret.Data["secret_id"]

	if !ok {
		return authToken{}, rootCAs, errors.New("Wrapped response is missing secret_id")
	}

	token, err := client.Logical().Write("auth/approle/login", map[string]interface{}{
		"role_id":   roleId,
		"secret_id": secretId,
	})

	if err != nil {
		return authToken{}, rootCAs, errors.Wrap(err, "could not log in using role_id and secret_id")
	}

	secretAuth := token.Auth

	return authToken{
		ClientToken:   secretAuth.ClientToken,
		Accessor:      secretAuth.Accessor,
		LeaseDuration: secretAuth.LeaseDuration,
		Renewable:     secretAuth.Renewable,
		VaultAddr:     wrappedSecretId.VaultAddr,
	}, rootCAs, nil
}

func generateCertificate(ip net.IP, duration time.Duration) (tls.Certificate, error) {

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	if err != nil {
		return tls.Certificate{}, errors.Wrap(err, "could not generate ECDSA key.")
	}

	notBefore := time.Now()

	notAfter := notBefore.Add(duration)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)

	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)

	if err != nil {
		return tls.Certificate{}, errors.Wrap(err, "failed to generate serial number")
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{ip},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)

	if err != nil {
		return tls.Certificate{}, errors.Wrap(err, "could not generate certificate")
	}

	certPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	b, err := x509.MarshalECPrivateKey(priv)

	if err != nil {
		return tls.Certificate{}, errors.Wrap(err, "could not marshal ECDSA private key")
	}

	keyPem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})

	cert, err := tls.X509KeyPair(certPem, keyPem)

	if err != nil {
		return tls.Certificate{}, errors.Wrap(err, "could not parse PEM certificate and private key")
	}

	return cert, nil
}
