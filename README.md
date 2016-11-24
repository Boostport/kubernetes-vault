# Kubernetes Vault Integration
The kubernetes-vault project allows pods to automatically receive a Vault token using Vault's [AppRole auth backend](https://www.vaultproject.io/docs/auth/approle.html).

![flow diagram](flow-diagram.png)

## Highlights
* Secure by default. The Kubernetes-Vault service does not allow using root tokens to authenticate against Vault.
* Prometheus metrics endpoint over http or https, with optional TLS client authentication.
* High availability mode using Raft, so that if the leader goes down, a follower can take over immediately.
* Peer discovery using Kubernetes services and endpoints and gossip to propagate peer changes across the cluster.

## Prerequisites:
* Vault should be 0.6.3 and above.
* You must use Kubernetes 1.4.0 and above as we rely on init containers (in beta) to accept the token.
* You must generate a periodic token with the correct policy to generate `secret_id`s using the AppRole backend.
* The Kubernetes-Vault service uses the Kubernetes service account to watch for new pods. This service account must have the appropriate permissions.
* Your app should use a [Vault client](https://www.vaultproject.io/docs/http/libraries.html) to renew the token and any secrets you request from Vault.
* You should configure Vault to use HTTPS, so that the authentication token and any other secrets cannot be sniffed.

## Get started
To run kubernetes-vault on your cluster, follow the [quick start guide](quick-start.md).

## Best practices
See our list of [best practices](best-practices.md).

## Configuration
The project consists of 2 containers, a service container what watches the Kubernetes cluster and pushes `secret_id`s to pods and an init container that
receives the `secret_id` and exchanges it for an auth token. These 2 containers are configured using environment variables. The init container also requires
configuration using Kubernetes annotations.

### Kubernetes-vault environment variables

| Environment Variable  | Description                                                               | Required   | Default Value                | Example                                 |
| RAFT_DIR              | Directory to store raft information.                                      | `no`       | `/var/lib/kubernetes-vault/` | `/var/my/dir`                           |
| VAULT_TOKEN           | Periodic Vault token to talk to Vault.                                    | `yes`      | `none`                       | `91526d9b-4850-3405-02a8-aa29e74e17a5`  |
| VAULT_ADDR            | Address of the Vault server.                                              | `yes`      | `none`                       | `https://vault:8200`                    |
| KUBERNETES_NAMESPACE  | The namespace the deployment runs in. Used to discover other nodes.       | `yes`      | `none`                       | `default`                               |
| KUBERNETES_SERVICE    | The service or headless service the deployment is in. Used for discovery. | `yes`      | `none`                       | `kubernetes-vault`                      |
| VAULT_CA_BACKEND      | The PKI backend to issue TLS certificates for the metrics endpoint.       | `no`       | `none`                       | `root-ca`                               |
| VAULT_CA_ROLE         | The Vault role of VAULT_CA_BACKEND to issue certificates.                 | `no`       | `none`                       | `kubernetes-vault`                      |
| VAULT_CLIENT_CAS      | A comma separated list of PKI backends to trust as Certificate roots.     | `no`       | `none`                       | `root-ca1,root-ca2`                     |

### Init container environment variables

| Environment Variable  | Description                                                 | Required  | Default Value                                | Example                                 |
| VAULT_ROLE_ID         | The Vault role id.                                          | `yes`     | `none`                                       | `313b0821-4ff6-1df8-54dd-c3eea5d3b8b1`  |
| TOKEN_PATH            | The location where the Vault token will be written          | `no`      | `/var/run/secrets/boostport.com/vault-token` | `/var/run/my/path`                      |

### Init container annotations

| Annotation                              | Description                         | Required  | Default Value | Example       |
| pod.boostport.com/vault-approle         | The Vault role.                     | `yes`     | `none`        | `sample-app`  |
| pod.boostport.com/vault-init-container  | The name of the init container.     | `yes`     | `none`        | `install`     |

## Metrics
Kubernetes-vault uses [Prometheus](https://prometheus.io) for metrics reporting. It exposes these metrics over the `/metrics` endpoint over http or https.

For more information about metrics, read the guide on [metrics](metrics.md).

## Development
PRs are highly welcomed!

We use glide as our dependency manager. To work on the project, install glide, then run `glide install --strip-vendor`.

The project also comes with a few scripts to simplify building binaries and docker containers and pushing docker containers.
Simply run `build.sh` to build the binaries. To build and push images, simplify run `build-images.sh`.
Running `build-images.sh` also automatically runs `build.sh`.

## License
This project is licensed under the Apache 2 License.