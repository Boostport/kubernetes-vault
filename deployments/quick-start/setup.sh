#!/bin/sh

# Check script dependencies
for command in http jq
do
    if ! hash $command 2>/dev/null; then
        echo $command is not available, please install $command
        exit 1
    fi
done

# Inspect the deployment file deployments/quick-start/vault.yaml. The deployment starts Vault in development mode with \
# the root token set to vault-root-token. It is also started using http. In production, you should run Vault over https.
kubectl apply -f vault.yaml

#2. Setup Vault
# Wait 5 minutes for vault to be deployed.
maxWaitSecond=10
vaultPod=$(kubectl get pods -l app=vault | grep ^vault* |awk '{print $1}')
while [ $maxWaitSecond -gt 0 ] && [ -z "$vaultPod" ]
do
    sleep 1
    echo waited 1 second for kubernetes to deploy Vault
    maxWaitSecond=$(($maxWaitSecond-1))
    vaultPod=$(kubectl get pod | grep ^vault* |awk '{print $1}')
done
if [ -z "$vaultPod" ]
then
    exit 1
fi

echo "Vault pod name: $vaultPod"
# Wait 10 seconds for vault to be running.
maxWaitSecond=300
vaultStatus=$(kubectl get pod "$vaultPod" -o=jsonpath='{.status.phase}')
while [ $maxWaitSecond -gt 0 ] && [ "$vaultStatus" != "Running" ]
do
    sleep 1
    echo waited 1 second for Vault up and running
    maxWaitSecond=$(($maxWaitSecond-1))
    vaultStatus=$(kubectl get pod "$vaultPod" -o=jsonpath='{.status.phase}')
    echo Vault pod status "$vaultStatus"
done
if [ "$vaultStatus" != "Running" ]
then
    exit 1
fi

# 2.1 Port forward vault
nohup kubectl port-forward $vaultPod 8200 &

# Set environment variables and authenticate
export VAULT_ADDR=http://127.0.0.1:8200
vault auth vault-root-token

# Set up the Root Certificate Authority
# Create a Root CA that expires in 10 years: 
vault mount -path=root-ca -max-lease-ttl=87600h pki

#Generate the root certificate: 
vault write root-ca/root/generate/internal common_name="Root CA" ttl=87600h exclude_cn_from_sans=true

# Set up the URLs: 
vault write root-ca/config/urls issuing_certificates="http://vault:8200/v1/root-ca/ca" \
    crl_distribution_points="http://vault:8200/v1/root-ca/crl"

# 2.3 Create the Intermediate Certificate Authority
# Create the Intermediate CA that expires in 5 years: 
vault mount -path=intermediate-ca -max-lease-ttl=43800h pki

# Generate a Certificate Signing Request: 
http POST http://127.0.0.1:8200/v1/intermediate-ca/intermediate/generate/internal X-Vault-Token:vault-root-token \
    common_name="Intermediate CA" ttl=43800h exclude_cn_from_sans=true | jq -r .data.csr > intermediate.csr

#Ask the Root to sign it: 
http POST http://127.0.0.1:8200/v1/root-ca/root/sign-intermediate X-Vault-Token:vault-root-token csr=@intermediate.csr \
    use_csr_values=true exclude_cn_from_sans=true | jq -r .data.certificate > signed.crt

# Send the stored certificate back to Vault: 
vault write intermediate-ca/intermediate/set-signed certificate=@signed.crt

#Set up URLs: 
vault write intermediate-ca/config/urls issuing_certificates="http://vault:8200/v1/intermediate-ca/ca" \
    crl_distribution_points="http://vault:8200/v1/intermediate-ca/crl"

#Create a role to allow Kubernetes-Vault to generate certificates: 
vault write intermediate-ca/roles/kubernetes-vault allow_any_name=true max_ttl="24h"

#2.4 Enable the AppRole backend

#Enable backend: 
vault auth-enable approle

#Set up an app-role for sample-app that generates a periodic 6 hour token: 
vault write auth/approle/role/sample-app secret_id_ttl=90s period=6h secret_id_num_uses=1

#2.5 Create token role for Kubernetes-Vault

#Inspect the policy file deployments/quick-start/policy.hcl

#Send the policy to Vault: 
vault policy-write kubernetes-vault policy.hcl

#Create a token role for Kubernetes-Vault that generates a 6 hour periodic token: 
vault write auth/token/roles/kubernetes-vault allowed_policies=kubernetes-vault period=6h

#2.6 Generate the token for Kubernetes-Vault and AppID

#Generate the token: 
#vault token-create -role=kubernetes-vault
CLIENTTOKEN=$(http POST http://127.0.0.1:8200/v1/auth/token/create/kubernetes-vault \
    X-Vault-Token:vault-root-token | jq -r .auth.client_token)
sed -i -e "s/token\: .*$/token: $CLIENTTOKEN/g" kubernetes-vault.yaml

#Get the AppID: 
#vault read auth/approle/role/sample-app/role-id
ROLEID=$(http GET http://127.0.0.1:8200/v1/auth/approle/role/sample-app/role-id \
    X-Vault-Token:vault-root-token | jq -r .data.role_id)
sed -i -e "s/value\"\: \".*$/value\": \"$ROLEID\"/g" sample-app.yaml

#3. Deploy Kubernetes-Vault

#3.1 Prepare the manifest and deploy

#Check deployments/quick-start/kubernetes-vault.yaml and update the Vault token in the Kubernetes deployment.
kubectl apply -f kubernetes-vault.yaml

#4. Deploy a sample app

#4.1 Prepare the manifest and deploy

#Inspect deployments/quick-start/sample-app.yaml and update the role id in the deployment.
kubectl apply -f sample-app.yaml

#5. Confirm that each pod of the sample app received a Vault token
echo View the logs using the Kubernetes dashboard or kubectl logs mypod and confirm that each pod receive a token. \
The token and various other information related to the token should be logged.