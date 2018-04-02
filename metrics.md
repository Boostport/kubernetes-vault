# Metrics

Kubernetes-Vault uses [Prometheus](https://www.prometheus.io) for metrics reporting. These metrics can be used for
monitoring and debugging. Metrics are not persisted, if a node restarts, the metrics for that node will reset.

To see the metrics, visit the `/metrics` endpoint.

## Configuration
Metrics can be served over http by default. To enable https, set the `VAULT_CA_BACKEND` and `VAULT_CA_ROLE` environment
variable. `VAULT_CA_BACKEND` is the PKI mount in Vault that we want to used. `VAULT_CA_ROLE` is the role for the PKI mount
that we want to use to issue TLS certificates.

If you need TLS Client Authentication (to ensure only authorized clients can connect to the metrics endpoint), set the
`VAULT_CLIENT_CAS` environment variable to a comma-separated list of Vault PKI mounts that will serve as your Root CAs.

## Collected metrics

### Kubernetes
These metrics are prefixed with `kubernetesvault_kubernetes_`.

| Name                   | Description                                                                 | Type  |
|------------------------|-----------------------------------------------------------------------------|-------|
| discovered_nodes_total | The total number of nodes discovered using the Kubernetes service endpoint. | Gauge |

### Vault
These metrics are prefixed with `kubernetesvault_vault_`.

| Name                                       | Description                                                                             | Type             |
|--------------------------------------------|-----------------------------------------------------------------------------------------|------------------|
| secret_id_requests_total                   | The total number of requests for an approle's secret_id.                                | Counter(AppRole) |
| secret_id_requests_failures_total          | The total number of requests for an approle's secret_id that failed.                    | Counter(AppRole) |
| token_renewal_requests_total               | The total number of requests to renew the auth token for Kubernetes-Vault.              | Counter          |
| token_renewal_request_failures_total       | The total number of requests to renew the auth token for Kubernetes-Vault that failed.  | Counter          |
| certificate_renewal_requests_total         | The total number of requests to renew the certificate for kubernetes-vault.             | Counter          |
| certificate_renewal_request_failures_total | The total number of requests to renew the certificate for kubernetes-vault that failed. | Counter          |

### Raft
These metrics are prefixed with `kubernetesvault_raft_`.

| Name                      | Description                                    | Type    |
|---------------------------|------------------------------------------------|---------|
| leader_changes_seen_total | The total number of leader changes seen.       | Counter |
| nodes_total               | The total number of raft nodes in the cluster. | Gauge   |

### Gossip
These metrics are prefixed with `kubernetesvault_gossip_`.

| Name               | Description                                                       | Type          |
|--------------------|-------------------------------------------------------------------|---------------|
| nodes_joined_total | The total number of times a node joined the cluster using gossip. | Counter(Node) |
| nodes_left_total   | The total number of times a node left the cluster using gossip.   | Counter(Node) |
| nodes_failed_total | The total number of times a gossip node failed.                   | Counter(Node) |
| nodes_reaped_total | The total number of times a gossip node was reaped.               | Counter(Node) |

### Server
These metrics are prefixed with `kubernetesvault_server_`.

| Name                       | Description                                     | Type             |
|----------------------------|-------------------------------------------------|------------------|
| secret_pushes_total        | The total number of secrets pushed.             | Counter(AppRole) |
| secret_push_failures_total | The total number of times a secret push failed. | Counter(AppRole) |
