# Best practices for running Kubernetes-Vault in production

* Vault should be configured to use https, so that secrets are secured.
* Do not provide Kubernetes-Vault with a root token. Instead, give it a periodic token that is heavily restricted (see `deployments/policy.hcl` for an example).
* Kubernetes-Vault expects a periodic token, so that it can renew the token without the token changing.
* For the AppRole backend, the `secret_id` should only have 1 use as each pod will get its own `secret_id`. The token generated from the AppRole should be periodic.
* Restrict each AppRole so that they only have access to secrets they really need. Policies for an AppRole can be updated by an administrator as requirements change.
* Secure the metrics endpoint to use https and enable TLS Client Authentication if required.
* Run multiple instances of Kubernetes-Vault, so that if the leader fails, another node can take over immediately.
