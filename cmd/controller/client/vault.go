package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/Boostport/kubernetes-vault/common"
	"github.com/cenkalti/backoff"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/vault/api"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

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

type RenewalConfig struct {
	initialTTL     int64
	counter        prometheus.Counter
	failureCounter prometheus.Counter
}

type renewalResult struct {
	ttl  int64
	data interface{}
}

type RootCAResolver interface {
	GetRootCAs() ([]byte, *x509.CertPool, error)
}

type VaultRootCAsResolver struct {
	Backends  []string
	VaultAddr string
}

func (v *VaultRootCAsResolver) GetRootCAs() ([]byte, *x509.CertPool, error) {
	certs := []byte{}
	buf := bytes.NewBuffer(certs)

	pool := x509.NewCertPool()

	httpClient := cleanhttp.DefaultPooledClient()
	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	httpClient.Transport.(*http.Transport).TLSClientConfig = tlsConfig

	for _, backend := range v.Backends {

		ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

		res, err := ctxhttp.Get(ctx, httpClient, fmt.Sprintf("%s/v1/%s/ca/pem", v.VaultAddr, backend))

		if err != nil {
			return buf.Bytes(), pool, errors.Wrapf(err, "could not get root certificate for the back end (%s)", backend)
		}

		defer res.Body.Close()

		pem, err := ioutil.ReadAll(res.Body)

		if err != nil {
			return buf.Bytes(), pool, errors.Wrap(err, "error reading response body")
		}

		_, err = buf.WriteString("\n")

		if err != nil {
			return buf.Bytes(), pool, errors.Wrap(err, "error writing new line between certificates to buffer")
		}

		_, err = buf.Write(pem)

		if err != nil {
			return buf.Bytes(), pool, errors.Wrap(err, "error writing certificate to buffer")
		}

		pool.AppendCertsFromPEM(pem)
	}

	return buf.Bytes(), pool, nil
}

type ExternalRootCAsResolver struct {
	CAFile string
}

func (e *ExternalRootCAsResolver) GetRootCAs() ([]byte, *x509.CertPool, error) {
	pool := x509.NewCertPool()

	certs, err := ioutil.ReadFile(e.CAFile)

	if err != nil {
		return certs, pool, errors.Wrapf(err, "could not read CA certificates from the file %s", e.CAFile)
	}

	pool.AppendCertsFromPEM(certs)

	return certs, pool, nil
}

type Vault struct {
	vaultAddr       string
	vaultRootCAs    []byte
	token           string
	skipTokenRoleNameValidation   	bool
	kubeServiceName string
	client          *api.Client
	tokenData       *tokenData
	logger          *logrus.Logger
	shutdown        chan struct{}
}

func (v *Vault) GetSecretId(role string) (common.WrappedSecretId, error) {

	s, err := v.client.Logical().Write(fmt.Sprintf("auth/approle/role/%s/secret-id", role), map[string]interface{}{})

	secretIdRequests.With(prometheus.Labels{"approle": role}).Inc()

	if err != nil {
		secretIdRequestFailures.With(prometheus.Labels{"approle": role}).Inc()
		return common.WrappedSecretId{}, errors.Wrap(err, "could not get secret_id")
	}

	return common.WrappedSecretId{
		SecretID:     s.WrapInfo.Token,
		CreationTime: s.WrapInfo.CreationTime,
		TTL:          s.WrapInfo.TTL,
		VaultAddr:    v.vaultAddr,
		VaultCAs:     v.vaultRootCAs,
	}, nil
}

func NewVault(vaultAddr string, token string, skipTokenRoleNameValidation bool, kubeServiceName string, wrappingTTL string, caResolver RootCAResolver, logger *logrus.Logger) (*Vault, error) {

	var (
		certs []byte
		roots *x509.CertPool
		err   error
	)

	if caResolver != nil {
		certs, roots, err = caResolver.GetRootCAs()

		if err != nil {
			return nil, errors.Wrap(err, "could not get vault root CAs")
		}
	}

	httpClient := cleanhttp.DefaultPooledClient()

	if roots != nil {
		tlsConfig := &tls.Config{RootCAs: roots}
		httpClient.Transport.(*http.Transport).TLSClientConfig = tlsConfig
	}

	client, err := api.NewClient(&api.Config{Address: vaultAddr, HttpClient: httpClient})

	if err != nil {
		return nil, errors.Wrap(err, "could not create vault client")
	}

	client.SetToken(token)

	v := &Vault{
		vaultAddr:       vaultAddr,
		vaultRootCAs:    certs,
		token:           token,
		skipTokenRoleNameValidation:	skipTokenRoleNameValidation,
		kubeServiceName: kubeServiceName,
		client:          client,
		logger:          logger,
		shutdown:        make(chan struct{}),
	}

	if err = v.parseToken(); err != nil {
		return nil, errors.Wrap(err, "error parsing supplied token")
	}

	v.client.SetWrappingLookupFunc(getWrappingFn(wrappingTTL))

	go v.renewToken()

	return v, nil
}

// getWrappingFn returns an appropriate wrapping function for Nomad Servers
func getWrappingFn(wrappingTTL string) func(operation, path string) string {

	return func(operation, path string) string {
		// Only wrap the token create operation
		if operation != "PUT" || !wrappedSecretIdRegex.MatchString(path) {
			return ""
		}

		return wrappingTTL
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

	if v.skipTokenRoleNameValidation == false {
		// There must be a valid role
		if data.Role == "" {
			multierror.Append(&mErr, errors.New("token role name must be set when not using a root token"))
		}

		if err := v.validateRole(data.Role); err != nil {
			multierror.Append(&mErr, err)
		}
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
		Period         int
		Renewable      bool
	}

	if err := mapstructure.WeakDecode(rsecret.Data, &data); err != nil {
		return errors.Wrap(err, "failed to parse Vault role's data block")
	}

	// Validate the role is acceptable
	var mErr multierror.Error

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

func (v *Vault) renew(renewalConfig RenewalConfig, renewal func() (renewalResult, error), success func(renewalResult), failure func(renewalResult, error)) {

	go func(initialTTL int64) {

		nextRenewal := time.Duration(initialTTL/2) * time.Second
		timer := time.NewTimer(nextRenewal)

		for {
			select {
			case <-timer.C:

				exp := backoff.NewExponentialBackOff()
				maxElapsedTime := calculateMaxElapsedTime(nextRenewal)
				exp.MaxElapsedTime = maxElapsedTime

				var (
					result renewalResult
					err    error
				)

				// Perform the renewal with backoff
				op := func() error {
					result, err = renewal()

					if err != nil {
						return errors.Wrap(err, "could not complete renewal operation")
					}

					return nil
				}

				err = backoff.Retry(op, exp)

				renewalConfig.counter.Inc()

				if err != nil {
					renewalConfig.failureCounter.Inc()
					failure(result, err)
					nextRenewal = 1 * time.Minute
				} else {
					success(result)
					nextRenewal = time.Duration(result.ttl/2) * time.Second
				}

				timer.Reset(nextRenewal)

			case <-v.shutdown:
				return
			}
		}

	}(renewalConfig.initialTTL)
}

func (v *Vault) renewToken() {

	renewalConfig := RenewalConfig{
		initialTTL:     int64(v.tokenData.TTL),
		counter:        tokenRenewalRequests,
		failureCounter: tokenRenewalFailures,
	}

	renewal := func() (renewalResult, error) {

		renewalResults := renewalResult{}

		s, err := v.client.Auth().Token().RenewSelf(0)

		if err != nil {
			return renewalResults, errors.Wrap(err, "error renewing vault token")
		}

		ttl := int64(s.Auth.LeaseDuration)
		renewalResults.ttl = ttl

		return renewalResults, nil
	}

	success := func(renewalResult renewalResult) {}

	failure := func(renewalResult renewalResult, err error) {
		v.logger.Errorf("Could not renew auth token: %s", err)
	}

	v.renew(renewalConfig, renewal, success, failure)
}

func (v *Vault) issueCertificate(ip net.IP, backend string, role string) (tls.Certificate, int, error) {

	serviceName := v.kubeServiceName

	hostname, err := os.Hostname()

	if err != nil {
		return tls.Certificate{}, 0, errors.Wrap(err, "could not lookup container hostname")
	}

	secret, err := v.client.Logical().Write(fmt.Sprintf("%s/issue/%s", backend, role), map[string]interface{}{
		"common_name": serviceName,
		"ip_sans":     ip.String(),
		"alt_names":   hostname,
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

	firstCert, err := x509.ParseCertificate(cert.Certificate[0])

	if err != nil {
		return tls.Certificate{}, 0, errors.Wrap(err, "could not parse certificate to get expiry")
	}

	duration := firstCert.NotAfter.Sub(time.Now())

	seconds := int(duration.Seconds())

	if seconds < 0 {
		return tls.Certificate{}, 0, errors.New("issued certificate is expired")
	}

	return cert, seconds, nil
}

func (v *Vault) GetAndRenewCertificate(ip net.IP, backend string, role string) (<-chan tls.Certificate, error) {

	ch := make(chan tls.Certificate, 8)

	cert, ttl, err := v.issueCertificate(ip, backend, role)

	if err != nil {
		return ch, errors.Wrap(err, "could not issue certificate")
	}

	ch <- cert

	renewal := func() (renewalResult, error) {

		renewalResults := renewalResult{}

		cert, ttl, err = v.issueCertificate(ip, backend, role)

		if err != nil {
			return renewalResults, err
		}

		renewalResults.ttl = int64(ttl)
		renewalResults.data = cert

		return renewalResults, nil
	}

	renewalConfig := RenewalConfig{
		initialTTL:     int64(ttl),
		counter:        certificateRenewalRequests,
		failureCounter: certificateRenewalFailures,
	}

	success := func(renewalResult renewalResult) {
		ch <- renewalResult.data.(tls.Certificate)
	}

	failure := func(renewalResult renewalResult, err error) {
		v.logger.Errorf("Could not renew certificate: %s", err)
	}

	v.renew(renewalConfig, renewal, success, failure)

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
