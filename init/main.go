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
	"path/filepath"
	"strings"
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

type secretID struct {
	RoleID    string `json:"roleId"`
	SecretID  string `json:"secretId"`
	Accessor  string `json:"accessor"`
	VaultAddr string `json:"vaultAddr"`
}

type wrappedSecretID struct {
	RoleID          string `json:"roleId"`
	WrappedSecretID string `json:"wrappedSecretId"`
	VaultAddr       string `json:"vaultAddr"`
	TTL             int    `json:"ttl"`
}

func main() {

	logLevel := os.Getenv("LOG_LEVEL")

	logger := logrus.New()
	logger.Level = logrus.DebugLevel

	if logLevel != "debug" && logLevel != "error" && logLevel != "" {
		logger.Fatalf(`Invalid LOG_LEVEL. Valid values are "debug" and "error".`)
	}

	if logLevel == "error" {
		logger.Level = logrus.ErrorLevel
	}

	roleID := os.Getenv("VAULT_ROLE_ID")

	if roleID == "" {
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

	retrieveToken := true

	retrieveAuthToken := os.Getenv("RETRIEVE_TOKEN")

	if strings.ToLower(retrieveAuthToken) == "false" {
		retrieveToken = false
	}


	unwrapSecret := true

	unwrapSecretId := os.Getenv("UNWRAP_SECRET")

	if strings.ToLower(unwrapSecretId) == "false" {
		unwrapSecret = false
	}

	if retrieveToken && !unwrapSecret {
		logger.Fatal("UNWRAP_SECRET cannot be false if RETRIEVE_TOKEN is true")
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

			if err := wrappedSecretId.Validate(); err != nil {
				logger.Fatalf("could not validate wrapped secret_id: %s", err)
			}

			var (
				response  interface{}
				tokenPath string
				err       error
			)

			if unwrapSecret {
				client, err := getAPIClient(wrappedSecretId.VaultAddr, wrappedSecretId.VaultCAs)

				if err != nil {
					logger.Fatalf("Error creating vault client: %s", err)
				}

				sID, secretIDAccessor, err := unwrapSecretID(client, wrappedSecretId.SecretID)

				if err != nil {
					logger.Fatalf("Could not unwrap secret: %s", err)
				}

				if retrieveToken {
					authToken, err := login(client, roleID, sID)

					if err != nil {
						logger.Fatalf("Could not login to get auth token: %s", err)
					}

					authToken.VaultAddr = wrappedSecretId.VaultAddr

					response = authToken

				} else {
					response = secretID{
						RoleID:    roleID,
						SecretID:  sID,
						Accessor:  secretIDAccessor,
						VaultAddr: wrappedSecretId.VaultAddr,
					}
				}

			} else {
				response = wrappedSecretID{
					RoleID: roleID,
					WrappedSecretID: wrappedSecretId.SecretID,
					VaultAddr: wrappedSecretId.VaultAddr,
					TTL: wrappedSecretId.TTL,
				}
			}

			b, err := json.Marshal(response)

			if err != nil {
				logger.Fatalf("Could not marshal auth token to JSON: %s", err)
			}

			if unwrapSecret {
				if retrieveToken {
					tokenPath = filepath.Join(credentialsPath, "vault-token")
				} else {
					tokenPath = filepath.Join(credentialsPath, "vault-secret-id")
				}
			} else {
				tokenPath = filepath.Join(credentialsPath, "vault-wrapped-secret-id")
			}

			err = ioutil.WriteFile(tokenPath, b, 0444)

			if err != nil {
				tokenType := ""
				if unwrapSecret {
					if retrieveToken {
						tokenType = "auth token"
					} else {
						tokenType = "secret_id"
					}
				} else {
					tokenType = "wrapped secret_id"
				}
				logger.Fatalf("Could not write %s to path (%s): %s", tokenType, tokenPath, err)
			}

			if len(wrappedSecretId.VaultCAs) > 0 {

				caBundlePath := filepath.Join(credentialsPath, "ca.crt")

				err = ioutil.WriteFile(caBundlePath, wrappedSecretId.VaultCAs, 0444)

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

func getAPIClient(vaultAddr string, rootCAs []byte) (*api.Client, error) {
	var roots *x509.CertPool

	if len(rootCAs) > 0 {
		roots = x509.NewCertPool()
		roots.AppendCertsFromPEM(rootCAs)
	}

	httpClient := cleanhttp.DefaultPooledClient()

	if roots != nil {
		tlsConfig := &tls.Config{RootCAs: roots}
		httpClient.Transport.(*http.Transport).TLSClientConfig = tlsConfig
	}

	return api.NewClient(&api.Config{Address: vaultAddr, HttpClient: httpClient})
}

func unwrapSecretID(client *api.Client, secretID string) (string, string, error) {
	client.SetToken(secretID)

	secret, err := client.Logical().Unwrap("")

	if err != nil {
		return "", "", errors.Wrap(err, "error unwrapping secret_id")
	}

	secretID, ok := secret.Data["secret_id"].(string)

	if !ok {
		return secretID, "", errors.New("Wrapped response is missing secret_id")
	}

	secretIDAccessor, ok := secret.Data["secret_id_accessor"].(string)

	if !ok {
		return secretID, secretIDAccessor, errors.New("Wrapped response is missing secret_id_accessor")
	}

	return secretID, secretIDAccessor, nil
}

func login(client *api.Client, roleID string, secretID string) (authToken, error) {

	token, err := client.Logical().Write("auth/approle/login", map[string]interface{}{
		"role_id":   roleID,
		"secret_id": secretID,
	})

	if err != nil {
		return authToken{}, errors.Wrap(err, "could not log in using role_id and secret_id")
	}

	secretAuth := token.Auth

	return authToken{
		ClientToken:   secretAuth.ClientToken,
		Accessor:      secretAuth.Accessor,
		LeaseDuration: secretAuth.LeaseDuration,
		Renewable:     secretAuth.Renewable,
	}, nil
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
