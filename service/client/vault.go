package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Boostport/kubernetes-vault/common"
	"github.com/Sirupsen/logrus"
	"github.com/cenkalti/backoff"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/vault/api"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"net"
	"os"
	"regexp"
	"time"
)

const wrappedSecretIdTTL = "60s"

var wrappedSecretIdRegex = regexp.MustCompile(`auth/approle/role/.+/secret-id`)

// tokenData holds the relevant information about the Vault token passed to the
// client.
type tokenData struct {
	CreationTTL int      `mapstructure:"creation_ttl"`
	TTL         int      `mapstructure:"ttl"`
	Renewable   bool     `mapstructure:"renewable"`
	Policies    []string `mapstructure:"policies"`
	Role        string   `mapstructure:"role"`
}

type Vault struct {
	vaultAddr string
	token     string
	client    *api.Client
	tokenData *tokenData
	logger    *logrus.Logger
	shutdown  chan struct{}
}

func (v *Vault) GetSecretId(role string) (common.WrappedSecretId, error) {

	s, err := v.client.Logical().Write(fmt.Sprintf("auth/approle/role/%s/secret-id", role), map[string]interface{}{})

	secretIdRequests.With(prometheus.Labels{"approle": role}).Inc()

	if err != nil {
		secretIdRequestFailures.With(prometheus.Labels{"approle": role}).Inc()
		return common.WrappedSecretId{}, errors.Wrap(err, "could not get secret_id")
	}

	return common.WrappedSecretId{
		Token:        s.WrapInfo.Token,
		CreationTime: s.WrapInfo.CreationTime,
		TTL:          s.WrapInfo.TTL,
		VaultAddr:    v.vaultAddr,
	}, nil
}

func NewVault(vaultAddr string, token string, logger *logrus.Logger) (*Vault, error) {

	client, err := api.NewClient(&api.Config{Address: vaultAddr, HttpClient: cleanhttp.DefaultPooledClient()})

	if err != nil {
		return nil, errors.Wrap(err, "could not create vault client")
	}

	client.SetToken(token)

	v := &Vault{
		vaultAddr: vaultAddr,
		token:     token,
		client:    client,
		logger:    logger,
		shutdown:  make(chan struct{}),
	}

	if err = v.parseToken(); err != nil {
		return nil, errors.Wrap(err, "error parsing supplied token")
	}

	v.client.SetWrappingLookupFunc(getWrappingFn())

	go v.renewToken()

	return v, nil
}

// getWrappingFn returns an appropriate wrapping function for Nomad Servers
func getWrappingFn() func(operation, path string) string {

	return func(operation, path string) string {
		// Only wrap the token create operation
		if operation != "PUT" || !wrappedSecretIdRegex.MatchString(path) {
			return ""
		}

		return wrappedSecretIdTTL
	}
}

func (v *Vault) parseToken() error {

	auth := v.client.Auth().Token()
	self, err := auth.LookupSelf()

	if err != nil {
		return errors.Wrap(err, "failed to lookup Vault periodic token")
	}

	// Read and parse the fields
	var data tokenData

	if err := mapstructure.WeakDecode(self.Data, &data); err != nil {
		return errors.Wrap(err, "failed to parse Vault token's data block")
	}

	for _, p := range data.Policies {
		if p == "root" {
			return errors.New("Do not use a root token. Use a token generated from a role instead.")
		}
	}

	var mErr multierror.Error

	// All tokens must be renewable
	if !data.Renewable {
		multierror.Append(&mErr, errors.New("vault token is not renewable"))
	}

	// All non-root tokens must have a lease duration
	if data.CreationTTL == 0 {
		multierror.Append(&mErr, errors.New("invalid lease duration of zero"))
	}

	// The lease duration can not be expired
	if data.TTL == 0 {
		multierror.Append(&mErr, errors.New("token TTL is zero"))
	}

	// There must be a valid role
	if data.Role == "" {
		multierror.Append(&mErr, errors.New("token role name must be set when not using a root token"))
	}

	if err := v.validateRole(data.Role); err != nil {
		multierror.Append(&mErr, err)
	}

	v.tokenData = &data

	return mErr.ErrorOrNil()
}

func (v *Vault) validateRole(role string) error {
	if role == "" {
		return errors.New("invalid empty role name")
	}

	// Validate the role
	rsecret, err := v.client.Logical().Read(fmt.Sprintf("auth/token/roles/%s", role))

	if err != nil {
		return errors.Wrapf(err, "failed to lookup role %s", role)
	}

	// Read and parse the fields
	var data struct {
		ExplicitMaxTtl int `mapstructure:"explicit_max_ttl"`
		Orphan         bool
		Period         int
		Renewable      bool
	}

	if err := mapstructure.WeakDecode(rsecret.Data, &data); err != nil {
		return errors.Wrap(err, "failed to parse Vault role's data block")
	}

	// Validate the role is acceptable
	var mErr multierror.Error

	if data.Orphan {
		multierror.Append(&mErr, errors.New("Role must not allow orphans"))
	}

	if !data.Renewable {
		multierror.Append(&mErr, errors.New("Role must allow tokens to be renewed"))
	}

	if data.ExplicitMaxTtl != 0 {
		multierror.Append(&mErr, errors.New("Role can not use an explicit max ttl. Token must be periodic."))
	}

	if data.Period == 0 {
		multierror.Append(&mErr, errors.New("Role must have a non-zero period to make tokens periodic."))
	}

	return mErr.ErrorOrNil()
}

func (v *Vault) renewToken() {

	nextRenewal := time.Duration(v.tokenData.TTL/2) * time.Second
	timer := time.NewTimer(nextRenewal)

	for {
		select {
		case <-timer.C:
			exp := backoff.NewExponentialBackOff()
			maxElapsedTime := calculateMaxElapsedTime(nextRenewal)
			exp.MaxElapsedTime = maxElapsedTime

			// Perform the renewal with backoff
			op := func() error {

				s, err := v.client.Auth().Token().RenewSelf(0)

				if err != nil {
					return errors.Wrap(err, "Error renewing vault token: %s")
				}

				ttl := int64(s.Auth.LeaseDuration)
				nextRenewal = time.Duration(ttl/2) * time.Second
				timer.Reset(nextRenewal)

				return nil
			}

			err := backoff.Retry(op, exp)

			tokenRenewalRequests.Inc()

			if err != nil {
				tokenRenewalFailures.Inc()
				v.logger.Infof("Could not renew auth token: %s", err)
			}

		case <-v.shutdown:
			return
		}
	}
}

func (v *Vault) issueCertificate(ip net.IP, backend string, role string) (tls.Certificate, int, error) {

	hostname, err := os.Hostname()

	if err != nil {
		return tls.Certificate{}, 0, errors.Wrap(err, "could not lookup container hostname")
	}

	secret, err := v.client.Logical().Write(fmt.Sprintf("%s/issue/%s", backend, role), map[string]interface{}{
		"common_name": hostname,
		"ip_sans":     ip.String(),
	})

	if err != nil {
		return tls.Certificate{}, 0, errors.Wrap(err, "error issuing certificate")
	}

	certs := secret.Data["certificate"].(string)

	if chain, ok := secret.Data["ca_chain"]; ok {

		for _, c := range chain.([]interface{}) {
			certs += "\n"
			certs += string(c.(string))
		}
	}

	key := secret.Data["private_key"].(string)

	cert, err := tls.X509KeyPair([]byte(certs), []byte(key))

	if err != nil {
		return tls.Certificate{}, 0, errors.Wrap(err, "could not parse certificate and private key")
	}

	return cert, secret.LeaseDuration, nil
}

func (v *Vault) GetAndRenewCertificate(ip net.IP, backend string, role string) (<-chan tls.Certificate, error) {

	ch := make(chan tls.Certificate, 8)

	cert, ttl, err := v.issueCertificate(ip, backend, role)

	if err != nil {
		return ch, errors.Wrap(err, "could not issue certificate")
	}

	ch <- cert

	go func(initialTTL int, ch chan<- tls.Certificate) {

		var (
			cert tls.Certificate
			ttl  int
			err  error
		)

		nextRenewal := time.Duration(float64(initialTTL)*0.75) * time.Second
		timer := time.NewTimer(nextRenewal)

		for {
			select {
			case <-timer.C:
				exp := backoff.NewExponentialBackOff()
				maxElapsedTime := calculateMaxElapsedTime(nextRenewal)
				exp.MaxElapsedTime = maxElapsedTime

				// Perform the renewal with backoff
				op := func() error {

					cert, ttl, err = v.issueCertificate(ip, backend, role)

					if err != nil {
						return err
					}

					return nil
				}

				err = backoff.Retry(op, exp)

				certificateRenewalRequests.Inc()

				if err != nil {
					certificateRenewalFailures.Inc()

					v.logger.Infof("Could not renew certificate: %s", err)
					nextRenewal = 1 * time.Minute
				} else {
					nextRenewal = time.Duration(float64(ttl/2)) * time.Second
				}

				timer.Reset(nextRenewal)

				ch <- cert

			case <-v.shutdown:
				return
			}
		}
	}(ttl, ch)

	return ch, nil
}

func (v *Vault) RootCertificates(roots []string) (*x509.CertPool, error) {

	pool := x509.NewCertPool()

	for _, root := range roots {

		s, err := v.client.Logical().Read(fmt.Sprintf("%s/cert/ca", root))

		if err != nil {
			return pool, errors.Wrap(err, "could not get root certificate")
		}

		pool.AppendCertsFromPEM([]byte(s.Data["certificate"].(string)))
	}

	return pool, nil
}

func (v *Vault) Shutdown() {
	close(v.shutdown)
}

// calculateMaxElapsedTime calculates the optimal maximum time for the backoff algorithm
func calculateMaxElapsedTime(t time.Duration) time.Duration {

	if t >= 10*time.Second {
		return t - (10 * time.Second)
	}

	return time.Duration(float64(t) * 0.5)
}
