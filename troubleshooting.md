# Troubleshooting
Here are some tips for troubleshooting and debugging issues when things aren't working.

## View init container logs
1. Enable debug logging on the init container by setting the `LOG_LEVEL` environment variable to
`debug`.

2. Assuming your init container is called `init-vault` and the pod is called `app-2539434469-99hz2`, 
run the following command in your CLI:

```
kubectl logs app-2539434469-99hz2 -c init-vault
```

## Connect Vault via https and ip. Error: certificate doesn't contain any IP SANs
Using Vault with certificate signed by unknown authority and accessing Vault via IP, kubernetes-vault controller may encounter an error:
```
time="2018-06-29T09:39:26Z" level=debug msg="Discovered 0 nodes: []"
time="2018-06-29T09:39:27Z" level=fatal msg="Could not create the vault client: error parsing supplied token: failed to lookup Vault periodic token: Get https://192.168.1.1:8200/v1/auth/token/lookup-self: x509: cannot validate certificate for 192.168.1.1 because it doesn't contain any IP SANs
```

This means that vault https certificate doesn't contain neccessary ip addresses in subject alternative names field.

1. Regenerate vault certificate adding ip addresses. I.e. for openssl add them in alt_names
```
[alt_names]
DNS.1 = vault.dns.1
DNS.2 = vault.dns.2
IP.1 = 192.168.1.1
IP.2 = 192.168.69.14
```
2. Add this certificate to vault.
3. Restart vault.
