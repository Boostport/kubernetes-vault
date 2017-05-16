#!/bin/sh
kubectl delete -f sample-app.yaml -f kubernetes-vault.yaml -f vault.yaml
ps -ef | grep "kubectl port-forward" | grep 8200 |awk '{print $2}'|xargs kill
rm intermediate.csr signed.crt nohup.out
