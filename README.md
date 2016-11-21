# Kubernetes Vault Integration
The kubernetes-vault project allows pods to automatically receive a Vault token using Vault's [AppRole auth backend](https://www.vaultproject.io/docs/auth/approle.html).

![flow diagram](flow-diagram.png)

# Prerequisites:
* You must use Kubernetes 1.4.0 and above as we rely on init containers (in beta) to accept the token.
* You must generate a periodic token with the correct policy to generate `secret_id`s using the AppRole backend. Root tokens are not accepted!
* The kubernetes-vault service uses a service account to watch for new pods. This service account must have the appropriate permissions.
* Your app should use a [vault client](https://www.vaultproject.io/docs/http/libraries.html) to renew the token and any secrets you request from Vault.
* You should configure Vault to use HTTPS, so that the authentication token and any other secrets cannot be sniffed.

# Test drive

# Limitations