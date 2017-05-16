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
This produces the following output:

```
$ vault write intermediate-ca/intermediate/generate/internal common_name="Intermediate CA" ttl=43800h exclude_cn_from_sans=true

Key     Value
---     -----
csr     -----BEGIN CERTIFICATE REQUEST-----
MIICXzCCAUcCAQAwGjEYMBYGA1UEAxMPSW50ZXJtZWRpYXRlIENBMIIBIjANBgkq
hkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAopGvAxQxiio5Mwqwl+Ri6PSahm1jMU/F
KV8qJzr7vrshmId3JuQUBHsaoIA2Ft27EIQyg9CUE0fcR/Ley+2Jzi4CiprH1xfo
UwHwI44e9LJ7PZYqUk6EemWhl/S3IyHxrezQfJQxWUmgKBlyXK9GfKFFQDKZ/+ts
GFw/733dAc3g7G/+w/oJ5SfA1aXP/YZynwHfLa8ni+bQmqTYdO4fCdtnP8aii7tx
kNLBHCMAtUv1PY8A+n5pknCzsCKd1s+GDM2A73USm+8evy+F+QTOXdFp9H1L0ryg
ShIdhXJdvEPOXtNBnbcTif5hAfUACC1Zij+8tLvlZWovkO9CXXl5CQIDAQABoAAw
DQYJKoZIhvcNAQELBQADggEBAFgC1PxhKwV3rzix1rockv2SCx+mjlTc2KW4MAcV
ZIPs0eWPpmXxdGCc0Zyi7kBbBjcb8ClHIFUmlxoR/qZiTSTa5nTyudL7+0/8ckit
kHbmVPxKadYtTF0RzwO+3031LAcRZzekhMxV8McHDRBoWGHObAgsn7UBsRunzdmD
gc0cxcbg6jSLQauO82lb+9OoJkZ+knrkCHrIgKoGl14N2/b4UyuIZ5t+CDltC9Ay
N4CmjVUt2fl+YRTSOH1ONr0WyF4swn7ifozd9UR6vWijXUAP4s1WSl5nk6A5N1nl
N0wLLG1lbWlo6hM2BvhC+mQAOB8pkwDO4z0cJvLfrfIP5BI=
-----END CERTIFICATE REQUEST-----
```

Copy the CSR (starting from `-----BEGIN CERTIFICATE REQUEST-----` until the end of `-----END CERTIFICATE REQUEST-----` from the output above) to a file called `intermediate.csr`:

```
-----BEGIN CERTIFICATE REQUEST-----
MIICXzCCAUcCAQAwGjEYMBYGA1UEAxMPSW50ZXJtZWRpYXRlIENBMIIBIjANBgkq
hkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAopGvAxQxiio5Mwqwl+Ri6PSahm1jMU/F
KV8qJzr7vrshmId3JuQUBHsaoIA2Ft27EIQyg9CUE0fcR/Ley+2Jzi4CiprH1xfo
UwHwI44e9LJ7PZYqUk6EemWhl/S3IyHxrezQfJQxWUmgKBlyXK9GfKFFQDKZ/+ts
GFw/733dAc3g7G/+w/oJ5SfA1aXP/YZynwHfLa8ni+bQmqTYdO4fCdtnP8aii7tx
kNLBHCMAtUv1PY8A+n5pknCzsCKd1s+GDM2A73USm+8evy+F+QTOXdFp9H1L0ryg
ShIdhXJdvEPOXtNBnbcTif5hAfUACC1Zij+8tLvlZWovkO9CXXl5CQIDAQABoAAw
DQYJKoZIhvcNAQELBQADggEBAFgC1PxhKwV3rzix1rockv2SCx+mjlTc2KW4MAcV
ZIPs0eWPpmXxdGCc0Zyi7kBbBjcb8ClHIFUmlxoR/qZiTSTa5nTyudL7+0/8ckit
kHbmVPxKadYtTF0RzwO+3031LAcRZzekhMxV8McHDRBoWGHObAgsn7UBsRunzdmD
gc0cxcbg6jSLQauO82lb+9OoJkZ+knrkCHrIgKoGl14N2/b4UyuIZ5t+CDltC9Ay
N4CmjVUt2fl+YRTSOH1ONr0WyF4swn7ifozd9UR6vWijXUAP4s1WSl5nk6A5N1nl
N0wLLG1lbWlo6hM2BvhC+mQAOB8pkwDO4z0cJvLfrfIP5BI=
-----END CERTIFICATE REQUEST-----
```

Ask the Root to sign it: `vault write root-ca/root/sign-intermediate csr=@intermediate.csr use_csr_values=true exclude_cn_from_sans=true`
This produces the following output:

```
$ vault write root-ca/root/sign-intermediate csr=@intermediate.csr use_csr_values=true exclude_cn_from_sans=true
Key             Value
---             -----
certificate     -----BEGIN CERTIFICATE-----
MIIDjzCCAnegAwIBAgIUfHs64iaYitI90rRdO1KwQ9aeyOwwDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAxMHUm9vdCBDQTAeFw0xNzA1MTYwMDMyMDNaFw0xNzA2MTcw
MDMyMzNaMBoxGDAWBgNVBAMTD0ludGVybWVkaWF0ZSBDQTCCASIwDQYJKoZIhvcN
AQEBBQADggEPADCCAQoCggEBANId94S+GpVpNeowu6FCnX9Cj2xB3Hlg1GZOk5aD
eeXtlZIRNJOh2CL5mAWW5z9uXVPrAyyLyv8XCoWW90Kv6Ug8vaQLpqPri/VrhnIR
QU5cNJyOPMxbI7FLVxczpPiC3N6bkeo3cb9xxQ047DP6lTuH4tQz27QM07gUwAqA
tfbT5/Lrhtp/t7AQFhAdtAjszt+m8ynoSKlrCex8JhhXFyqcEWHiMXJArezBha5A
RMSWg+GOL3+93SAmoxvicNAMux66ySh7+IOUMrsq2mz1r0TeAxrBZ+6fLg9ueauo
M7X1J1+Y2xX6f2dcO1+bACJFq46WZzwPR6XmUKUaixchbKECAwEAAaOB1DCB0TAO
BgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQULgYM0Bme
2U0illjmjKoKyJNK/qEwHwYDVR0jBBgwFoAUQjqA35amUd+9ydGMtwhplzzB4fww
OwYIKwYBBQUHAQEELzAtMCsGCCsGAQUFBzAChh9odHRwOi8vdmF1bHQ6ODIwMC92
MS9yb290LWNhL2NhMDEGA1UdHwQqMCgwJqAkoCKGIGh0dHA6Ly92YXVsdDo4MjAw
L3YxL3Jvb3QtY2EvY3JsMA0GCSqGSIb3DQEBCwUAA4IBAQAr81mn6k+BOKU6vAdT
WGAs4y6GlAh6Sh1VSa14lVLm8TmRFm+RkbJB0AoXGRSLC+PHGPtQmDMMC7iu0mWv
dHeneIMECeUrWQ4A7zw3LfezdeaFKMCi+/zygTrhA57USw2a4vGMfHcJUgF30ewp
drFA35yrwy2J7zZLxN5ZKb9KdfOkKydD+NeqXuyUg03Rd/HyvSoH6lUMPM+Oa6rQ
E7aeEQJEjW3EIfAeFGpN1UDBL0zpSYP3b2N+/6PMmEZ6+ZZjgq23w5KY5o6HoJUU
zvSpIV8rSQNZp1h6L1X30KJ93Np+LNsn7IWrJYqVAgrvPV+rz3IirqxvcsHYC+Jz
d8eY
-----END CERTIFICATE-----
expiration      1497659553
issuing_ca      -----BEGIN CERTIFICATE-----
MIIC9DCCAdygAwIBAgIUeZecI1XCafhE33cMK02+dYaFBeswDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAxMHUm9vdCBDQTAeFw0xNzA1MTYwMDI5MDhaFw0yNzA1MTQw
MDI5MzhaMBIxEDAOBgNVBAMTB1Jvb3QgQ0EwggEiMA0GCSqGSIb3DQEBAQUAA4IB
DwAwggEKAoIBAQCjhyNVQVjkCNTgfi1QYS7vdV68BrgnIFD41Evu6AKHiyeMFIho
0JbBOdC8xbiaxdSbL4ogbryXR3RywcFi34YBmtP+R92O1o6P5tZCokP7Fv9DLy4b
EZHCIJ4MxZJogmy/NvuNUGVJG586qj/r7eYCvxxhnUoUGa5+r4S6RdBQV1N507r8
A3gt7rpE7XOZeBRGCdCuy34L2jExOrzKzs0mXmEarN4mH+c0z6d7Km7+g3Ww1gB1
w0PZek1IxUf047L+UlRKdoFhU+Zz1R+ZggqbzRYPkKfvRA80Bm1jEBq4YI6ywuvy
N51cecOKpHeW7LyPeCqPs/6ovJSOyi6DKePFAgMBAAGjQjBAMA4GA1UdDwEB/wQE
AwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBRCOoDflqZR373J0Yy3CGmX
PMHh/DANBgkqhkiG9w0BAQsFAAOCAQEAkVF52gJ0+1UhDMhXfWb7uBTtUqKCmlLQ
sLqq2NLwG1VB1NLN2iYSdngo4JAJHZhUqGsmsyIkJCQYvtguJiY6rcHmPz0aGL7X
L+9/T68EGXvyflEwjfWfrc4yvQHVte3f7/uABj0y+APPPstrkCW52zUwcTvtzmlg
Df2h00s1lw/gLrgFGFVHMp71wqNj7KV45VJtE6RSpC+OkPDsHiD5Hojo/VLrE5zF
gQ6YBNJ8lnJu8Hm2WRBy4WhknYbqrb2mEZq6IkPf/XWwi3KHf+TpZ0kYvcs866As
RuOGDW9aPsu4DLuSL9494F1XId0XAuiREiJoiHwzo0U/MLIFZWtLTg==
-----END CERTIFICATE-----
serial_number   7c:7b:3a:e2:26:98:8a:d2:3d:d2:b4:5d:3b:52:b0:43:d6:9e:c8:ec
```

Copy the certificate (under the `certificate` key in the above output, from `-----BEGIN CERTIFICATE-----` until the end of `-----END CERTIFICATE-----`) to `signed.crt`:

```
-----BEGIN CERTIFICATE-----
MIIDjzCCAnegAwIBAgIUfHs64iaYitI90rRdO1KwQ9aeyOwwDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAxMHUm9vdCBDQTAeFw0xNzA1MTYwMDMyMDNaFw0xNzA2MTcw
MDMyMzNaMBoxGDAWBgNVBAMTD0ludGVybWVkaWF0ZSBDQTCCASIwDQYJKoZIhvcN
AQEBBQADggEPADCCAQoCggEBANId94S+GpVpNeowu6FCnX9Cj2xB3Hlg1GZOk5aD
eeXtlZIRNJOh2CL5mAWW5z9uXVPrAyyLyv8XCoWW90Kv6Ug8vaQLpqPri/VrhnIR
QU5cNJyOPMxbI7FLVxczpPiC3N6bkeo3cb9xxQ047DP6lTuH4tQz27QM07gUwAqA
tfbT5/Lrhtp/t7AQFhAdtAjszt+m8ynoSKlrCex8JhhXFyqcEWHiMXJArezBha5A
RMSWg+GOL3+93SAmoxvicNAMux66ySh7+IOUMrsq2mz1r0TeAxrBZ+6fLg9ueauo
M7X1J1+Y2xX6f2dcO1+bACJFq46WZzwPR6XmUKUaixchbKECAwEAAaOB1DCB0TAO
BgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQULgYM0Bme
2U0illjmjKoKyJNK/qEwHwYDVR0jBBgwFoAUQjqA35amUd+9ydGMtwhplzzB4fww
OwYIKwYBBQUHAQEELzAtMCsGCCsGAQUFBzAChh9odHRwOi8vdmF1bHQ6ODIwMC92
MS9yb290LWNhL2NhMDEGA1UdHwQqMCgwJqAkoCKGIGh0dHA6Ly92YXVsdDo4MjAw
L3YxL3Jvb3QtY2EvY3JsMA0GCSqGSIb3DQEBCwUAA4IBAQAr81mn6k+BOKU6vAdT
WGAs4y6GlAh6Sh1VSa14lVLm8TmRFm+RkbJB0AoXGRSLC+PHGPtQmDMMC7iu0mWv
dHeneIMECeUrWQ4A7zw3LfezdeaFKMCi+/zygTrhA57USw2a4vGMfHcJUgF30ewp
drFA35yrwy2J7zZLxN5ZKb9KdfOkKydD+NeqXuyUg03Rd/HyvSoH6lUMPM+Oa6rQ
E7aeEQJEjW3EIfAeFGpN1UDBL0zpSYP3b2N+/6PMmEZ6+ZZjgq23w5KY5o6HoJUU
zvSpIV8rSQNZp1h6L1X30KJ93Np+LNsn7IWrJYqVAgrvPV+rz3IirqxvcsHYC+Jz
d8eY
-----END CERTIFICATE-----
```

Send the stored certificate back to Vault: `vault write intermediate-ca/intermediate/set-signed certificate=@signed.crt`

Set up URLs: `vault write intermediate-ca/config/urls issuing_certificates="http://vault:8200/v1/intermediate-ca/ca" crl_distribution_points="http://vault:8200/v1/intermediate-ca/crl"`

Create a role to allow Kubernetes-Vault to generate certificates: `vault write intermediate-ca/roles/kubernetes-vault allow_any_name=true max_ttl="24h"`

### 2.4 Enable the AppRole backend
Enable backend: `vault auth-enable approle`

Set up an app-role for `sample-app` that generates a periodic 6 hour token: `vault write auth/approle/role/sample-app secret_id_ttl=90s period=6h secret_id_num_uses=1`

### 2.5 Create token role for Kubernetes-Vault
Inspect the policy file `deployments/quick-start/policy.hcl`

Send the policy to Vault: `vault policy-write kubernetes-vault deployments/quick-start/policy.hcl`

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

- As we did not define a policy on the AppRole, the `sample-app`'s token will give it access to Vault using the `default`
policy. In production, you should set appropriate policies for the app-role when [creating it](https://www.vaultproject.io/docs/auth/approle.html#auth-approle-role-role_name-).

- In production, you will most likely have multiple apps and microservices running in your Kubernetes cluster. In this case,
you SHOULD have a separate AppRole for each app with appropriate policies to restrict access to the secret tree for each app.
Simply create as many AppRoles as required and set the appropriate `app-id` in the Kubernetes-Vault init-container for each app.

## 6. Tear down
Clean up: `kubectl delete -f deployments/quick-start/sample-app.yaml -f deployments/quick-start/kubernetes-vault.yaml`

## Further deployment options
In this guide, we did not set up TLS client authentication for the metrics endpoint. To do so, simply set the `vaultCABackends`
or `caCert` in the `prometheus.tls` configuration.

## Best Practices
See our documented [best practices](best-practices.md).
