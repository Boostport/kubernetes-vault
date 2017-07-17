package metrics

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	certificateCh <-chan tls.Certificate

	sync.Mutex
	certificate *tls.Certificate
}

func (s *Server) getCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return s.certificate, nil
}

func (s *Server) watchForNewCertificates() {
	for {
		select {
		case cert := <-s.certificateCh:
			s.certificate = &cert
		}
	}
}

func (s *Server) start(roots *x509.CertPool) {

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Handler: mux,
	}

	if s.certificateCh != nil {
		tlsConfig := &tls.Config{
			GetCertificate: s.getCertificate,
		}

		if roots != nil {
			tlsConfig.ClientCAs = roots
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}

		server.TLSConfig = tlsConfig

		go server.ListenAndServeTLS("", "")
	} else {
		go server.ListenAndServe()
	}
}

func StartServer(certificateCh <-chan tls.Certificate, roots *x509.CertPool) {

	server := &Server{
		certificateCh: certificateCh,
	}

	if certificateCh != nil {
		go server.watchForNewCertificates()
	}

	go server.start(roots)
}
