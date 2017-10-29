#!/bin/sh

KUBERNETES_VAULT_DEPLOYMENT="kubernetes-vault.yaml"

if [ "$KUBE_1_5" = true ]; then
    KUBERNETES_VAULT_DEPLOYMENT="kubernetes-vault-kube-1.5.yaml"
fi

SAMPLE_APP_DEPLOYMENT="sample-app.yaml"

if [ "$KUBE_1_5" = true ]; then
    SAMPLE_APP_DEPLOYMENT="sample-app-kube-1.5.yaml"
fi

kubectl delete -f "$SAMPLE_APP_DEPLOYMENT" -f "$KUBERNETES_VAULT_DEPLOYMENT" -f vault.yaml
pid=$(pgrep -f "kubectl port-forward" | grep 8200)
if [ ! -z "$pid" ]; then
    kill "$pid"
fi
rm -f nohup.out
