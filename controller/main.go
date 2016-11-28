package main

import (
	"crypto/x509"
	"github.com/Boostport/kubernetes-vault/common"
	"github.com/Boostport/kubernetes-vault/controller/client"
	"github.com/Boostport/kubernetes-vault/controller/cluster"
	"github.com/Boostport/kubernetes-vault/controller/metrics"
	"github.com/Sirupsen/logrus"
	"github.com/kubernetes/client-go/pkg/util/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {

	logger := logrus.New()
	logger.Level = logrus.DebugLevel

	raftDir := os.Getenv("RAFT_DIR")

	if raftDir == "" {
		raftDir = "/var/lib/kubernetes-vault/"
	}

	err := os.MkdirAll(raftDir, 0666)

	if err != nil {
		logger.Fatalf("Error while trying to create raft directory (%s): %s", raftDir, err)
	}

	bindAddr, err := common.ExternalIP()

	if err != nil {
		logger.Fatalf("Could not determine external ip address: %s", err)
	}

	vaultToken := os.Getenv("VAULT_TOKEN")

	if vaultToken == "" {
		logger.Fatal("The VAULT_TOKEN environment variable is not set.")
	}

	vaultAddr := os.Getenv("VAULT_ADDR")

	if vaultAddr == "" {
		logger.Fatal("The VAULT_ADDR environment variable is not set.")
	}

	vaultCAs := os.Getenv("VAULT_CA_BACKENDS")

	certBackend := os.Getenv("CERT_BACKEND")

	certRole := os.Getenv("CERT_ROLE")

	if (certRole != "") != (certBackend != "") {
		logger.Fatalf("The CERT_BACKEND and CERT_ROLE environment variables must both be provided if you want to serve the metrics endpoint over https.")
	}

	prometheusCAs := os.Getenv("PROMETHEUS_CA_BACKENDS")

	if certRole == "" && certBackend == "" && prometheusCAs != "" {
		logger.Fatalf("The CERT_BACKEND and CERT_ROLE environment variables must be set if you want to use PROMETHEUS_CA_BACKENDS.")
	}

	kubeNamespace := os.Getenv("KUBERNETES_NAMESPACE")

	if kubeNamespace == "" {
		logger.Fatal("The KUBERNETES_NAMESPACE environment variable is not set.")
	}

	kubeService := os.Getenv("KUBERNETES_SERVICE")

	if kubeService == "" {
		logger.Fatal("The KUBERNETES_SERVICE environment variable is not set.")
	}

	kube, err := client.NewKube(kubeNamespace)

	if err != nil {
		logger.Fatalf("Could not create the kubernetes client: %s", err)
	}

	// Wait between 3 and 10 seconds before discovering other nodes
	time.Sleep(time.Duration(rand.Intn(7)+3) * time.Second)

	nodes, err := kube.Discover(kubeService)

	if err != nil {
		logger.Fatalf("Error while discovering nodes: %s", err)
	}

	logger.Debugf("Discovered %d nodes: %s", len(nodes), nodes)

	vault, err := client.NewVault(vaultAddr, vaultToken, strings.Split(vaultCAs, ","), logger)

	if err != nil {
		logger.Fatalf("Could not create the vault client: %s", err)
	}

	if certBackend != "" && certRole != "" {

		certCh, err := vault.GetAndRenewCertificate(bindAddr, certBackend, certRole)

		if err != nil {
			logger.Fatalf("Could not get certificate for metrics server: %s", err)
		}

		var roots *x509.CertPool

		if prometheusCAs != "" {

			roots, err = vault.RootCertificates(strings.Split(prometheusCAs, ","))

			if err != nil {
				logger.Fatalf("Could not get root certificates: %s", err)
			}
		}

		metrics.StartServer(certCh, roots)

	} else {
		metrics.StartServer(nil, nil)
	}

	gossip, err := cluster.NewGossip(bindAddr.String(), nodes, 0, logger.Writer())

	if err != nil {
		logger.Fatalf("Could not create gossip: %s", err)
	}

	storeConfig := cluster.DefaultStoreConfig()
	storeConfig.Logger = logger

	store := cluster.NewStore(gossip, kube, vault, storeConfig)

	err = store.StartRaft(raftDir, bindAddr.String(), logger.Writer())

	if err != nil {
		logger.Fatalf("Could not start raft: %s", err)
	}

	sigs := make(chan os.Signal, 1)
	done := make(chan struct{}, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		store.Shutdown()
		vault.Shutdown()
		done <- struct{}{}
	}()

	<-done
}
