package cluster

import "github.com/prometheus/client_golang/prometheus"

var (
	leaderChangesSeen = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "raft",
		Name:      "leader_changes_seen_total",
		Help:      "The total number of leader changes seen.",
	})

	nodesTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "kubernetesvault",
		Subsystem: "raft",
		Name:      "nodes_total",
		Help:      "The total number of raft nodes in the cluster.",
	})

	nodeJoined = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "gossip",
		Name:      "nodes_joined_total",
		Help:      "The total number of times a node joined the cluster using gossip.",
	},
		[]string{"node"},
	)

	nodeLeft = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "gossip",
		Name:      "nodes_left_total",
		Help:      "The total number of times a node left the cluster using gossip.",
	},
		[]string{"node"},
	)

	nodeFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "gossip",
		Name:      "nodes_failed_total",
		Help:      "The total number of times a gossip node failed.",
	},
		[]string{"node"},
	)

	nodeReaped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "gossip",
		Name:      "nodes_reaped_total",
		Help:      "The total number of times a gossip node was reaped.",
	},
		[]string{"node"},
	)

	secretPushes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "server",
		Name:      "secret_pushes_total",
		Help:      "The total number of secrets pushed.",
	},
		[]string{"approle"},
	)

	secretPushFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubernetesvault",
		Subsystem: "server",
		Name:      "secret_push_failures_total",
		Help:      "The total number of times a secret push failed.",
	},
		[]string{"approle"},
	)
)

func init() {
	prometheus.MustRegister(leaderChangesSeen)
	prometheus.MustRegister(nodesTotal)
	prometheus.MustRegister(nodeJoined)
	prometheus.MustRegister(nodeLeft)
	prometheus.MustRegister(nodeFailed)
	prometheus.MustRegister(nodeReaped)
	prometheus.MustRegister(secretPushes)
	prometheus.MustRegister(secretPushFailures)
}
