package client

import "github.com/prometheus/client_golang/prometheus"

var (
	kubeDiscoveredNodes = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "kubernetesvault",
		Subsystem: "kubernetes",
		Name:      "discovered_nodes_total",
		Help:      "The total number of nodes discovered using the kubernetes endpoint.",
	})

	secretIdRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "vault",
		Name:      "secret_id_requests_total",
		Help:      "The total number of requests for an approle's secret_id.",
	},
		[]string{"approle"},
	)

	secretIdRequestFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "vault",
		Name:      "secret_id_requests_failures_total",
		Help:      "The total number of requests for an approle's secret_id that failed.",
	},
		[]string{"approle"},
	)

	tokenRenewalRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "vault",
		Name:      "token_renewal_requests_total",
		Help:      "The total number of requests to renew the auth token for kubernetes-vault.",
	})

	tokenRenewalFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "vault",
		Name:      "token_renewal_request_failures_total",
		Help:      "The total number of requests to renew the auth token for kubernetes-vault that failed.",
	})

	certificateRenewalRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "vault",
		Name:      "certificate_renewal_requests_total",
		Help:      "The total number of requests to renew the certificate for kubernetes-vault.",
	})

	certificateRenewalFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "vault",
		Name:      "certificate_renewal_request_failures_total",
		Help:      "The total number of requests to renew the certificate for kubernetes-vault that failed.",
	})
)

func init() {
	prometheus.MustRegister(kubeDiscoveredNodes)
	prometheus.MustRegister(secretIdRequests)
	prometheus.MustRegister(secretIdRequestFailures)
	prometheus.MustRegister(tokenRenewalRequests)
	prometheus.MustRegister(tokenRenewalFailures)
	prometheus.MustRegister(certificateRenewalRequests)
	prometheus.MustRegister(certificateRenewalFailures)
}
