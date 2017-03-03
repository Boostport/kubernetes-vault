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