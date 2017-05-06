# Quick start guide

## Prerequisites
* You have a working Kubernetes cluster and it is at least v1.4.0.
* You have a basic understanding of how Kubernetes and Vault works.

## 1. Deploy Vault
Inspect the deployment file `deployments/quick-start/vault.yaml`. The deployment starts Vault in development mode with the root token
set to `vault-root-token`. It is also started using `http`. In production, you should run Vault over `https`.

Deploy: `kubectl apply -f deployments/quick-start/vault.yaml`

## 2. Setup Vault
### 2.1 Port forward vault
Substitute the appropriate pod name for Vault: `kubectl port-forward vault-361162082-kuufw 8200`

### 2.2 Set environment variables and authenticate
`set VAULT_ADDR=http://127.0.0.1:8200` for Windows

`export VAULT_ADDR=http://127.0.0.1:8200` for Linux

Type in the root token (`vault-root-token`) to authenticate: `vault auth`

### 2.2 Set up the Root Certificate Authority
Create a Root CA that expires in 10 years: `vault mount -path=root-ca -max-lease-ttl=87600h pki`

Generate the root certificate: `vault write root-ca/root/generate/internal common_name="Root CA" ttl=87600h exclude_cn_from_sans=true`

Set up the URLs: `vault write root-ca/config/urls issuing_certificates="http://vault:8200/v1/root-ca/ca" crl_distribution_points="http://vault:8200/v1/root-ca/crl"`

### 2.3 Create the Intermediate Certificate Authority
Create the Intermediate CA that expires in 5 years: `vault mount -path=intermediate-ca -max-lease-ttl=43800h pki`

Generate a Certificate Signing Request: `vault write intermediate-ca/intermediate/generate/internal common_name="Intermediate CA" ttl=43800h exclude_cn_from_sans=true`

Copy the CSR to a test file called `intermediate.csr`

Ask the Root to sign it: `vault write root-ca/root/sign-intermediate csr=@intermediate.csr use_csr_values=true exclude_cn_from_sans=true`

Copy the certificate to `signed.crt`

Send the stored certificate back to Vault: `vault write intermediate-ca/intermediate/set-signed certificate=@signed.crt`

Set up URLs: `vault write intermediate-ca/config/urls issuing_certificates="http://vault:8200/v1/intermediate-ca/ca" crl_distribution_points="http://vault:8200/v1/intermediate-ca/crl"`

Create a role to allow Kubernetes-Vault to generate certificates: `vault write intermediate-ca/roles/kubernetes-vault allow_any_name=true max_ttl="24h"`

### 2.4 Enable the AppRole backend
Enable backend: `vault auth-enable approle`

Set up an app-role for `sample-app` that generates a periodic 6 hour token: `vault write auth/approle/role/sample-app secret_id_ttl=90s period=6h secret_id_num_uses=1`

### 2.5 Create token role for Kubernetes-Vault
Inspect the policy file `deployments/quick-start/policy.hcl`

Send the policy to Vault: `vault policy-write kubernetes-vault policy.hcl`

Create a token role for Kubernetes-Vault that generates a 6 hour periodic token: `vault write auth/token/roles/kubernetes-vault allowed_policies=kubernetes-vault period=6h`

### 2.6 Generate the token for Kubernetes-Vault and AppID
Generate the token: `vault token-create -role=kubernetes-vault`. And make a note of the token output. In the example below it would be '00000000-1111-2222-3333-444444444444'

```
$ vault token-create -role=kubernetes-vault

Key             Value
---             -----
token           00000000-1111-2222-3333-444444444444      
token_accessor  c50fb600-aaaa-bbbb-cccc-xxxxxxxxxxxx
token_duration  6h0m0s
token_renewable true
token_policies  [default kubernetes-vault]
```

Get the app's role id: `vault read auth/approle/role/sample-app/role-id`
```
$ vault read auth/approle/role/sample-app/role-id

Key     Value
---     -----
role_id zzzzzzzzz-7777-8888-9999-tttttttttttt
```

## 3. Deploy Kubernetes-Vault
### 3.1 Prepare the manifest and deploy
Check `deployments/quick-start/kubernetes-vault.yaml` and update the Vault token (not the role id) in the Kubernetes deployment.

For example:
```
....
----
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernetes-vault
data:
  kubernetes-vault.yml: |-
    vault:
      addr: http://vault:8200
      token: "00000000-1111-2222-3333-444444444444"
...
```

Deploy: `kubectl apply -f deployments/quick-start/kubernetes-vault.yaml`

### 3.2 Confirm Kubernetes-Vault deployed successfully
Use the Kubernetes dashboard to view the status of the deployment and make sure all pods are healthy.

## 4. Deploy a sample app
### 4.1 Prepare the manifest and deploy
Inspect `deployments/quick-start/sample-app.yaml` and update the role id in the deployment:

```
...
spec:
  replicas: 5
  template:
    metadata:
      labels:
        app: sample-app
      annotations:
        pod.boostport.com/vault-approle: sample-app
        pod.boostport.com/vault-init-container: install
        pod.beta.kubernetes.io/init-containers: '[
          {
            "name": "install",
            "image": "boostport/kubernetes-vault-init:0.4.4",
            "imagePullPolicy": "Always",
            "env": [
                {
                    "name": "VAULT_ROLE_ID",
                    "value": "zzzzzzzzz-7777-8888-9999-tttttttttttt"
                }
            ],
            "volumeMounts": [
                {
                    "name": "vault-token",
                    "mountPath": "/var/run/secrets/boostport.com"
                }
            ]
          }
        ]'
...
```

Deploy: `kubectl apply -f deployments/quick-start/sample-app.yaml`

## 5. Confirm that each pod of the sample app received a Vault token
View the logs using the Kubernetes dashboard or `kubectl logs mypod` and confirm that each pod receive a token.
The token and various other information related to the token should be logged.

## 6. Tear down
Clean up: `kubectl delete -f deployments/quick-start/sample-app.yaml -f deployments/quick-start/kubernetes-vault.yaml`

## Further deployment options
In this guide, we did not set up TLS client authentication for the metrics endpoint. To do so, simply set the `vaultCABackends`
or `caCert` in the `prometheus.tls` configuration.

## Best Practices
See our documented [best practices](best-practices.md).
