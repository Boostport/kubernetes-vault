path "intermediate-ca/issue/kubernetes-vault" {
  capabilities = ["update"]
}

path "auth/token/roles/kubernetes-vault" {
  capabilities = ["read"]
}
