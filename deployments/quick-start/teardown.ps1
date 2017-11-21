param(
    [switch] $KUBE_1_5
)
if ($KUBE_1_5 -eq $true) {
    $KUBERNETES_VAULT_DEPLOYMENT="kubernetes-vault-kube-1.5.yaml"
    $SAMPLE_APP_DEPLOYMENT = "sample-app-kube-1.5.yaml"
}
else {
    $KUBERNETES_VAULT_DEPLOYMENT="kubernetes-vault.yaml"
    $SAMPLE_APP_DEPLOYMENT = "sample-app.yaml"
}

Invoke-Expression "kubectl delete -f vault.yaml; kubectl delete -f $($KUBERNETES_VAULT_DEPLOYMENT); kubectl delete -f $($SAMPLE_APP_DEPLOYMENT)"

$kubectl_processes = Get-WmiObject win32_process -filter "name like 'kubectl%'"
foreach ($process in $kubectl_processes) {
    if ($process.CommandLine.EndsWith(8200)) {
        Stop-Process -Id $process.handle
    }
}
