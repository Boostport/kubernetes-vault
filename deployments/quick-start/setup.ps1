# Set up parameters to determine if this is Kubernetes 1.5 or later

param(
    [switch] $KUBE_1_5
)

# Check for command dependencies
$commands = @("kubectl", "vault")

# Loop over the commands
foreach ($command in $commands) {
    if ((Get-Command $command -ErrorAction SilentlyContinue) -eq $null) {
        Write-Host "Unable to find $($command) in your PATH."
        # If the command isn't found, let's try installing it via Choco
        if (Get-Command "choco" -ErrorAction SilentlyContinue) {
            # Kubectl is contained in the kubernetes-cli Choco package, so let's swap the command name.
            if ($command -eq "kubectl") {
                $command = "kubernetes-cli"
            }
            Write-Host "Installing $($command) with Chocolatey."
            # Start an administrative shell to install the tool
            $torun = "choco install -y $($command)"
            $installproc = Start-Process -FilePath powershell.exe -ArgumentList "-NoProfile -ExecutionPolicy Bypass -command $torun" -verb RunAs -WorkingDirectory C: -PassThru
            # Let's wait for the install to finish before continuing
            $installproc.WaitForExit()
            # And make sure that the command should be in the PATH
            RefreshEnv
        }
        else {
            Write-Host "We also could not install $($command) via Chocolatey."
            Write-Host "Either install Chocolatey or $($command) in your PATH"
            exit 1
        }
    }
}

# Apply the Vault definition
kubectl apply -f vault.yaml

# We may need to update the Pod details more than once, so let's make it a function
function Find-VaultPod {
    $k8s_pods = kubectl get pod -o json | ConvertFrom-Json
    foreach ($pod in $k8s_pods.items) {
        if ($pod.metadata.name.StartsWith("vault")) {
            return $pod
        }
    }
}

# Wait for the Pod to be created
$vault_pod = $null
while ($vault_pod -eq $null) {
    $vault_pod = Find-VaultPod
    if ($vault_pod -ne $null) {
        Write-Host "Vault pod name: $($vault_pod.metadata.name)"
    }
    else {
        Write-Host "Vault not started. Waiting 1 second."
        Start-Sleep -Seconds 1
        $vault_pod = Find-VaultPod
    }
}

# And if the Pod isn't running, let's wait for it to become running.
while ($vault_pod.status.phase -ne "Running") {
    Write-Host "$($vault_pod.metadata.name) is $($vault_pod.status.phase). Waiting 1 second for it to become Running."
    Start-Sleep -Seconds 1
    $vault_pod = Find-VaultPod
}


# Run a background job to set up Port Forwarding to Vault
$pf_job = "kubectl port-forward $($vault_pod.metadata.name) 8200"
Write-Host "This can occasionally hang. Pressing an arrow key with the kubectl window open will let the process continue."
$proxy = Start-Process -FilePath powershell.exe -ArgumentList "-NoProfile -ExecutionPolicy Bypass -command $pf_job" -PassThru

# Set the Vault Address environment variable
[Environment]::SetEnvironmentVariable("VAULT_ADDR", "http://127.0.0.1:8200", "Process")

# Authenticat to Vault
vault login vault-root-token

# Set up the Root Certificate Authority
# Create a Root CA that expires in 10 years:
vault secrets enable -path=root-ca -max-lease-ttl=87600h pki

# Generate the root certificate:
vault write root-ca/root/generate/internal common_name="Root CA" ttl=87600h exclude_cn_from_sans=true

# Set up the URLs:
vault write root-ca/config/urls issuing_certificates="http://vault:8200/v1/root-ca/ca" crl_distribution_points="http://vault:8200/v1/root-ca/crl"

# Create the Intermediate Certificate Authority
# Create the Intermediate CA that expires in 5 years:
vault secrets enable -path=intermediate-ca -max-lease-ttl=43800h pki

# Generate a Certificate Signing Request:
$intermediate_csr_response = vault write -format=json intermediate-ca/intermediate/generate/internal common_name="Intermediate CA" ttl=43800h exclude_cn_from_sans=true

$intermediate_csr = "$($intermediate_csr_response)" | ConvertFrom-Json

$csr_data = $intermediate_csr.data.csr

# Ask the Root to sign it:
$intermediate_cert_response = vault write -format=json root-ca/root/sign-intermediate csr=$csr_data use_csr_values=true exclude_cn_from_sans=true

$signed_cert = "$($intermediate_cert_response)" | ConvertFrom-Json
$signed_data = $signed_cert.data.certificate

# Send the signed certificate back to Vault:
vault write intermediate-ca/intermediate/set-signed certificate=$signed_data

# Set up URLs:
vault write intermediate-ca/config/urls issuing_certificates="http://vault:8200/v1/intermediate-ca/ca" crl_distribution_points="http://vault:8200/v1/intermediate-ca/crl"

# Enable the AppRole backend
vault auth enable approle

# Create a role to allow Kubernetes-Vault to generate certificates:
vault write intermediate-ca/roles/kubernetes-vault allow_any_name=true max_ttl="24h"

# Inspect the policy file deployments/quick-start/policy-kubernetes-vault.hcl

# Send the policy to Vault:
vault policy write kubernetes-vault policy-kubernetes-vault.hcl

# Create a token role for Kubernetes-Vault that generates a 6 hour periodic token:
vault write auth/token/roles/kubernetes-vault allowed_policies=kubernetes-vault period=6h

# Generate a client token
$client_token_response = vault token-create -format=json -role=kubernetes-vault

$client_token_data = "$($client_token_response)" | ConvertFrom-Json

$CLIENT_TOKEN = $client_token_data.auth.client_token

# Prepare the manifest
if ($KUBE_1_5 -eq $true) {
    $KUBERNETES_VAULT_DEPLOYMENT="kubernetes-vault-kube-1.5.yaml"
}
else {
    $KUBERNETES_VAULT_DEPLOYMENT="kubernetes-vault.yaml"
}

Write-Host "Setting Client Token to $($CLIENT_TOKEN)"
$temp = Get-Content $KUBERNETES_VAULT_DEPLOYMENT
$temp -replace 'token: .*',"token: $($CLIENT_TOKEN)" | Out-File $KUBERNETES_VAULT_DEPLOYMENT

# Deploy
kubectl apply -f $KUBERNETES_VAULT_DEPLOYMENT

# Sample app

# Set up an app-role

# Set up an app-role for sample-app that generates a periodic 6 hour token:
vault write auth/approle/role/sample-app secret_id_ttl=90s period=6h secret_id_num_uses=1 policies=kubernetes-vault,default

# Add new rules to kubernetes-vault policy
$current_rules_response = vault read -format=json sys/policy/kubernetes-vault
$current_rules_data = "$($current_rules_response)" | ConvertFrom-Json
$current_rules = $current_rules_data.data.rules
$app_rules = Get-Content "policy-sample-app.hcl"

"$($current_rules)`n`n$($app_rules)" | vault write sys/policy/kubernetes-vault policy=-

# Prepare the manifest and deploy the app

# Get the app’s role id:
$role_id_response = vault read -format=json auth/approle/role/sample-app/role-id
$role_id_data = "$($role_id_response)" | ConvertFrom-Json
$ROLEID = $role_id_data.data.role_id
Write-Host "Deploying Sample App with Role ID $($ROLEID)"

# Inspect deployments/quick-start/sample-app.yaml and update the role id in the deployment

$SAMPLE_APP_DEPLOYMENT = $null

if ($KUBE_1_5 -eq $true) {
    $SAMPLE_APP_DEPLOYMENT = "sample-app-kube-1.5.yaml"    
}
else {
    $SAMPLE_APP_DEPLOYMENT = "sample-app.yaml"
}

$temp = Get-Content $SAMPLE_APP_DEPLOYMENT

if ($KUBE_1_5 -eq $true) {
    $temp -replace 'value\": .*',"value`": $($ROLEID)`"" | Out-File $SAMPLE_APP_DEPLOYMENT
}
else {
    $temp -replace 'value: .*',"value: $($ROLEID)" | Out-File $SAMPLE_APP_DEPLOYMENT
}
# Deploy
kubectl apply -f $SAMPLE_APP_DEPLOYMENT

# 4. Confirm that each pod of the sample app received a Vault token
Write-Host "`n`nView the logs using the Kubernetes dashboard or kubectl logs mypod
and confirm that each pod receive a token. The token and various other
information related to the token should be logged.`n"

