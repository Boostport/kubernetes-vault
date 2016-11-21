package main

import (
	"github.com/Boostport/kubernetes-vault/common"
	"github.com/Boostport/kubernetes-vault/service/client"
	"github.com/Boostport/kubernetes-vault/service/cluster"
	"github.com/Sirupsen/logrus"
	"github.com/kubernetes/client-go/pkg/util/rand"
	"os"
	"os/signal"
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

	// Wait between 0 and 5 seconds before discovering other nodes
	time.Sleep(time.Duration(rand.Intn(5)) * time.Second)

	nodes, err := kube.Discover(kubeService)

	if err != nil {
		logger.Fatalf("Error while discovering nodes: %s", err)
	}

	logger.Debugf("Discovered %d nodes: %s", len(nodes), nodes)

	vault, err := client.NewVault(vaultAddr, vaultToken, logger)

	if err != nil {
		logger.Fatalf("Could not create the vault client: %s", err)
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
