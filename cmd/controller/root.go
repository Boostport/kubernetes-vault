package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Boostport/kubernetes-vault/cmd/controller/client"
	"github.com/Boostport/kubernetes-vault/cmd/controller/cluster"
	"github.com/Boostport/kubernetes-vault/cmd/controller/metrics"
	"github.com/Boostport/kubernetes-vault/common"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const defaultWrappingTTL = "60s"

func init() {
	RootCmd.Flags().String("config", "", "Path to the configuration file. By default, this is kubernetes-vault.yml in the current working directory.")
	RootCmd.Flags().String("log-level", "debug", `Log verbosity. Defaults to "debug" and written to stdout and stderr. Supported values: "debug", "error"`)
}

type config struct {
	RaftDir string `mapstructure:"raftDir"`

	Vault struct {
		Addr  string `mapstructure:"addr"`
		Token string `mapstructure:"token"`
		SkipTokenRoleNameValidation bool `mapstructure:"skipTokenRoleNameValidation"`
		TLS   struct {
			VaultCABackends []string `mapstructure:"vaultCABackends"`
			CACert          string   `mapstructure:"caCert"`
		} `mapstructure:"tls"`
		WrappingTTL string `mapstructure:"wrappingTTL"`
	} `mapstructure:"vault"`

	Kubernetes struct {
		WatchNamespace   string `mapstructure:"watchNamespace"`
		ServiceNamespace string `mapstructure:"serviceNamespace"`
		Service          string `mapstructure:"service"`
	} `mapstructure:"kubernetes"`

	Prometheus struct {
		TLS struct {
			VaultCertBackend string   `mapstructure:"vaultCertBackend"`
			VaultCertRole    string   `mapstructure:"vaultCertRole"`
			VaultCABackends  []string `mapstructure:"VaultCABackends"`
			CertFile         string   `mapstructure:"certFile"`
			CertKey          string   `mapstructure:"certKey"`
			CACert           string   `mapstructure:"caCert"`
		} `mapstructure:"tls"`
	} `mapstructure:"prometheus"`
}

func (c *config) Validate() error {

	var errs error

	hasTLSConfig := c.Prometheus.TLS.VaultCertBackend != "" || c.Prometheus.TLS.VaultCertRole != "" || c.Prometheus.TLS.CertFile != "" || c.Prometheus.TLS.CertKey != ""
	hasBothVaultCAAndExternalCAConfig := (c.Prometheus.TLS.VaultCertBackend != "" || c.Prometheus.TLS.VaultCertRole != "") == (c.Prometheus.TLS.CertFile != "" || c.Prometheus.TLS.CertKey != "")

	if hasTLSConfig && hasBothVaultCAAndExternalCAConfig {
		errs = multierror.Append(errs, errors.New(`Contraditory TLS configuration. You must use either Vault (tls.vaultCertBackend and tls.vaultCertRole) or your own certificate files (tls.certFilePath and tls.certKeyPath) to manage the TLS certificate, not both.`))
	} else {
		if (c.Prometheus.TLS.VaultCertBackend != "") != (c.Prometheus.TLS.VaultCertRole != "") {
			errs = multierror.Append(errs, errors.New("Both prometheus.tls.vaultCertBackend and prometheus.tls.vaultCertRole must be provided if you want to serve the metrics endpoint over https using Vault as your certificate authority."))
		}

		if (c.Prometheus.TLS.CertFile != "") != (c.Prometheus.TLS.CertKey != "") {
			errs = multierror.Append(errs, errors.New("Both prometheus.tls.certFile and prometheus.tls.certKey must be provided if you want to serve the metrics endpoint over https using your own certificate."))
		}
	}

	if c.Vault.Addr == "" {
		errs = multierror.Append(errs, errors.New("vault.addr is required"))
	}

	if c.Vault.Token == "" {
		errs = multierror.Append(errs, errors.New("vault.token is required"))
	}

	if len(c.Vault.TLS.VaultCABackends) > 0 && c.Vault.TLS.CACert != "" {
		errs = multierror.Append(errs, errors.New(`Contraditory Vault TLS configuration. You must use either Vault CA backends (vault.tls.vaultCABackends) or your own Root CA file (vault.tls.caCertFilePath) to verify the Vault server TLS certificate, not both.`))
	}

	if c.Kubernetes.WatchNamespace == "" {
		errs = multierror.Append(errs, errors.New("kubernetes.watchNamespace is required"))
	}

	if c.Kubernetes.ServiceNamespace == "" {
		errs = multierror.Append(errs, errors.New("kubernetes.serviceNamespace is required"))
	}

	if c.Kubernetes.Service == "" {
		errs = multierror.Append(errs, errors.New("kubernetes.service is required"))
	}

	if len(c.Prometheus.TLS.VaultCABackends) > 0 && c.Prometheus.TLS.CACert != "" {
		errs = multierror.Append(errs, errors.New(`Contraditory Prometheus TLS configuration. You use either Vault CA backends (prometheus.tls.vaultCABackends) or your own Root CA file (prometheus.tls.caCertFilePath) to verify the Prometheus scraper TLS certificate, not both.`))
	}

	hasPrometheusRootCAs := len(c.Prometheus.TLS.VaultCABackends) > 0 || c.Prometheus.TLS.CACert != ""
	hasVaultCAOrExternalCAConfig := (c.Prometheus.TLS.VaultCertBackend != "" && c.Prometheus.TLS.VaultCertRole != "") || (c.Prometheus.TLS.CertFile != "" && c.Prometheus.TLS.CertKey != "")

	if hasPrometheusRootCAs && !hasVaultCAOrExternalCAConfig {
		errs = multierror.Append(errs, errors.New("You need to set a Vault certificate backend ( prometheus.tls.vaultCertBackend and prometheus.tls.vaultCertRole) or use an external certificate (prometheus.tls.certFile and prometheus.tls.certKey) if you want to secure the prometheus endpoint with client-side certificate authentication."))
	}

	return errs
}

func newConfigWithDefaults() *config {
	cfg := &config{
		RaftDir: "/var/lib/kubernetes-vault/",
	}

	cfg.Vault.WrappingTTL = defaultWrappingTTL

	return cfg
}

func certificateFromFile(certFile string, keyFile string) (<-chan tls.Certificate, error) {

	ch := make(chan tls.Certificate, 1)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)

	if err != nil {
		return ch, errors.Wrap(err, "could not load certificate")
	}

	ch <- cert

	return ch, nil
}

var RootCmd = &cobra.Command{
	Use:   "kubernetes-vault",
	Short: "Kubernetes-vaults is a Kubernetes controller that pushes Vault tokens into pods",
	Long: `Kubernetes-vault is a Kubernetes controller that watches new pods for certain annotations
		and pushes a wrapped secret into an init container in the pod. The init container exchanges
		the secret and the pod's configured AppRole with Vault for a token and writes the token
		to a shared volume for the pod. More information at https://github.com/Boostport/kubernetes-vault`,
	Run: func(cmd *cobra.Command, args []string) {

		logger := logrus.New()
		logger.Level = logrus.DebugLevel

		logLevel := strings.ToLower(cmd.Flags().Lookup("log-level").Value.String())

		if logLevel != "debug" && logLevel != "error" {
			logger.Fatalf(`logLevel should be either "debug" or "error", got "%s"`, logLevel)
		}

		if logLevel == "error" {
			logger.Level = logrus.ErrorLevel
		}

		if c := cmd.Flags().Lookup("config").Value.String(); c != "" {
			viper.SetConfigFile(c)
		} else {
			viper.AddConfigPath(".")
			viper.SetConfigName("kubernetes-vault")
		}

		err := viper.ReadInConfig()

		if err != nil {
			logger.Fatalf("Error reading config file: %s", err)
		}

		conf := newConfigWithDefaults()

		err = viper.Unmarshal(conf)

		if err != nil {
			logger.Fatalf("Error processing config file: %s", err)
		}

		conf = expandEnvironmentVariables(conf)

		err = conf.Validate()

		if err != nil {
			logger.Fatalf("Invalid config file: %s", err)
		}

		err = os.MkdirAll(conf.RaftDir, 0666)

		if err != nil {
			logger.Fatalf("Error while trying to create raft directory (%s): %s", conf.RaftDir, err)
		}

		bindAddr, err := common.ExternalIP()

		if err != nil {
			logger.Fatalf("Could not determine external ip address: %s", err)
		}

		kube, err := client.NewKube(conf.Kubernetes.WatchNamespace, logger)

		if err != nil {
			logger.Fatalf("Could not create the kubernetes client: %s", err)
		}

		// Wait between 3 and 10 seconds before discovering other nodes
		time.Sleep(time.Duration(rand.Intn(7)+3) * time.Second)

		nodes, err := kube.Discover(conf.Kubernetes.ServiceNamespace, conf.Kubernetes.Service)

		if err != nil {
			logger.Fatalf("Error while discovering nodes: %s", err)
		}

		logger.Debugf("Discovered %d nodes: %s", len(nodes), nodes)

		var rootCAResolver client.RootCAResolver

		if len(conf.Vault.TLS.VaultCABackends) > 0 {

			rootCAResolver = &client.VaultRootCAsResolver{
				Backends:  conf.Vault.TLS.VaultCABackends,
				VaultAddr: conf.Vault.Addr,
			}

		} else if conf.Vault.TLS.CACert != "" {

			rootCAResolver = &client.ExternalRootCAsResolver{
				CAFile: conf.Vault.TLS.CACert,
			}
		}

		vault, err := client.NewVault(conf.Vault.Addr, conf.Vault.Token, conf.Vault.SkipTokenRoleNameValidation, conf.Kubernetes.Service, conf.Vault.WrappingTTL, rootCAResolver, logger)

		if err != nil {
			logger.Fatalf("Could not create the vault client: %s", err)
		}

		var certCh <-chan tls.Certificate

		if conf.Prometheus.TLS.VaultCertBackend != "" && conf.Prometheus.TLS.VaultCertRole != "" {

			certCh, err = vault.GetAndRenewCertificate(bindAddr, conf.Prometheus.TLS.VaultCertBackend, conf.Prometheus.TLS.VaultCertRole)

			if err != nil {
				logger.Fatalf("Could not get Vault certificate for metrics server: %s", err)
			}
		}

		if conf.Prometheus.TLS.CertFile != "" && conf.Prometheus.TLS.CertKey != "" {
			certCh, err = certificateFromFile(conf.Prometheus.TLS.CertFile, conf.Prometheus.TLS.CertKey)

			if err != nil {
				logger.Fatalf("Could not load certificate for metrics server: %s", err)
			}
		}

		var roots *x509.CertPool

		if len(conf.Prometheus.TLS.VaultCABackends) > 0 {

			roots, err = vault.RootCertificates(conf.Prometheus.TLS.VaultCABackends)

			if err != nil {
				logger.Fatalf("Could not get root certificates from Vault: %s", err)
			}
		}

		if conf.Prometheus.TLS.CACert != "" {
			roots = x509.NewCertPool()

			p, err := ioutil.ReadFile(conf.Prometheus.TLS.CACert)

			if err != nil {
				logger.Fatalf("Could not read CA certificates from the file (%s): %s", conf.Prometheus.TLS.CACert, err)
			}

			roots.AppendCertsFromPEM(p)
		}

		metrics.StartServer(certCh, roots)

		gossip, err := cluster.NewGossip(bindAddr.String(), nodes, 0, logger.WriterLevel(logrus.DebugLevel))

		if err != nil {
			logger.Fatalf("Could not create gossip: %s", err)
		}

		storeConfig := cluster.DefaultStoreConfig()
		storeConfig.Logger = logger

		store := cluster.NewStore(gossip, kube, vault, storeConfig)

		err = store.StartRaft(conf.RaftDir, bindAddr.String(), logger.WriterLevel(logrus.DebugLevel))

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
	},
}
