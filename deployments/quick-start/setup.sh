#!/bin/sh

# Check script dependencies
for command in jq kubectl vault
do
    if ! hash $command 2>/dev/null; then
        echo $command is not available, please install $command
        exit 1
    fi
done

# 1. Vault

# 1.1. Deploy
# Inspect the deployment file deployments/quick-start/vault.yaml. The deployment starts Vault in development mode with
# the root token set to vault-root-token. It is also started using http. In production, you should run Vault over https.
kubectl apply -f vault.yaml

# Wait 10 seconds for vault to be deployed.
maxWaitSecond=10
vaultPod=$(kubectl get pods -l app=vault | grep "^vault*" |awk '{print $1}')
while [ $maxWaitSecond -gt 0 ] && [ -z "$vaultPod" ]
do
    sleep 1
    echo waited 1 second for kubernetes to deploy Vault
    maxWaitSecond=$((maxWaitSecond-1))
    vaultPod=$(kubectl get pod | grep "^vault*" |awk '{print $1}')
done
if [ -z "$vaultPod" ]
then
    exit 1
fi

echo "Vault pod name: $vaultPod"
# Wait 5 minutes for vault to be running.
maxWaitSecond=300
vaultStatus=$(kubectl get pod "$vaultPod" -o=jsonpath='{.status.phase}')
while [ $maxWaitSecond -gt 0 ] && [ "$vaultStatus" != "Running" ]
do
    sleep 1
    echo waited 1 second for Vault up and running
    maxWaitSecond=$((maxWaitSecond-1))
    vaultStatus=$(kubectl get pod "$vaultPod" -o=jsonpath='{.status.phase}')
    echo Vault pod status "$vaultStatus"
done
if [ "$vaultStatus" != "Running" ]
then
    exit 1
fi

# 1.2. Port forward
nohup kubectl port-forward "$vaultPod" 8200 &
echo "Waiting for port forwarding to start"
sleep 3

# 1.3. Set environment variables and authenticate
export VAULT_ADDR=http://127.0.0.1:8200
vault login vault-root-token

# 1.4. Set up the Root Certificate Authority
# Create a Root CA that expires in 10 years:
vault secrets enable -path=root-ca -max-lease-ttl=87600h pki

# Generate the root certificate:
vault write root-ca/root/generate/internal common_name="Root CA" ttl=87600h exclude_cn_from_sans=true

# Set up the URLs:
vault write root-ca/config/urls issuing_certificates="http://vault:8200/v1/root-ca/ca" \
    crl_distribution_points="http://vault:8200/v1/root-ca/crl"

# 1.5. Create the Intermediate Certificate Authority
# Create the Intermediate CA that expires in 5 years:
vault secrets enable -path=intermediate-ca -max-lease-ttl=43800h pki

# Generate a Certificate Signing Request:
vault write -format=json intermediate-ca/intermediate/generate/internal \
    common_name="Intermediate CA" ttl=43800h exclude_cn_from_sans=true \
    | jq -r .data.csr > intermediate.csr

# Ask the Root to sign it:
vault write -format=json root-ca/root/sign-intermediate \
    csr=@intermediate.csr use_csr_values=true exclude_cn_from_sans=true format=pem_bundle \
    | jq -r .data.certificate | sed -e :a -e '/^\n*$/{$d;N;};/\n$/ba' > signed.crt
rm -f intermediate.csr

# Send the stored certificate back to Vault:
vault write intermediate-ca/intermediate/set-signed certificate=@signed.crt
rm -f signed.crt

# Set up URLs:
vault write intermediate-ca/config/urls issuing_certificates="http://vault:8200/v1/intermediate-ca/ca" \
    crl_distribution_points="http://vault:8200/v1/intermediate-ca/crl"

# 1.6. Enable the AppRole backend
vault auth enable approle

# 2. Kubernetes-Vault

# 2.1. Create roles and policies for Kubernetes-Vault

# Create a role to allow Kubernetes-Vault to generate certificates:
vault write intermediate-ca/roles/kubernetes-vault allow_any_name=true max_ttl="24h"

# Inspect the policy file deployments/quick-start/policy-kubernetes-vault.hcl

# Send the policy to Vault:
vault policy write kubernetes-vault policy-kubernetes-vault.hcl

# Create a token role for Kubernetes-Vault that generates a 6 hour periodic token:
vault write auth/token/roles/kubernetes-vault allowed_policies=kubernetes-vault period=6h

# 2.2. Generate the token for Kubernetes-Vault and AppID

# Generate the token:
CLIENTTOKEN=$(vault token create -format=json -role=kubernetes-vault | jq -r .auth.client_token)

# 2.3. Prepare the manifest and deploy

KUBERNETES_VAULT_DEPLOYMENT="kubernetes-vault.yaml"

if [ "$KUBE_1_5" = true ]; then
    KUBERNETES_VAULT_DEPLOYMENT="kubernetes-vault-kube-1.5.yaml"
fi

sed -i -e "s/token\: .*$/token: $CLIENTTOKEN/g" $KUBERNETES_VAULT_DEPLOYMENT

# Deploy
kubectl apply -f $KUBERNETES_VAULT_DEPLOYMENT

# 3. Sample app

# 3.1. Set up an app-role

# Set up an app-role for sample-app that generates a periodic 6 hour token:
vault write auth/approle/role/sample-app secret_id_ttl=90s period=6h secret_id_num_uses=1 policies=kubernetes-vault,default

# 3.2. Add new rules to kubernetes-vault policy
current_rules="$(vault read -format=json sys/policy/kubernetes-vault | jq -r .data.rules)"
app_rules="$(cat policy-sample-app.hcl)"
printf "%s\n\n%s" "$current_rules" "$app_rules" | vault write sys/policy/kubernetes-vault policy=-

# 3.3. Prepare the manifest and deploy the app

# Get the app’s role id:
ROLEID=$(vault read -format=json auth/approle/role/sample-app/role-id | jq -r .data.role_id)

# Inspect deployments/quick-start/sample-app.yaml and update the role id in the deployment

SAMPLE_APP_DEPLOYMENT="sample-app.yaml"

if [ "$KUBE_1_5" = true ]; then
    SAMPLE_APP_DEPLOYMENT="sample-app-kube-1.5.yaml"
    sed -i -e "s/value\"\: \".*$/value\": \"$ROLEID\"/g" $SAMPLE_APP_DEPLOYMENT
else
    sed -i -e "s/value\: .*$/value: $ROLEID/g" $SAMPLE_APP_DEPLOYMENT
fi

# Deploy
kubectl apply -f $SAMPLE_APP_DEPLOYMENT

# 4. Confirm that each pod of the sample app received a Vault token
printf "\nView the logs using the Kubernetes dashboard or kubectl logs mypod
and confirm that each pod receive a token. The token and various other
information related to the token should be logged.\n"
