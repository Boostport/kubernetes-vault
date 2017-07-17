#!/usr/bin/env sh

CONFIG_FILE=/kubernetes-vault/kubernetes-vault.yml

# Write the configuration if it was passed in using the KUBERNETES_VAULT_LOCAL_CONFIG environment variable
if [ -n "$KUBERNETES_VAULT_LOCAL_CONFIG" ]; then
    echo "$KUBERNETES_VAULT_LOCAL_CONFIG" > "$CONFIG_FILE"
fi

NUM_ARGS=$#

# If the user is trying to run kuberentes-vault directly with some or no arguments, then
# pass them to kubernetes-vault.
if [ "${1:0:1}" = '-' ] || [ $NUM_ARGS = 0 ]; then
    set -- kubernetes-vault "$@"
fi

HAS_CONFIG_FLAG=false

for i in "$@" ; do
    if [[ "${i:0:8}" == "--config" ]] ; then
        HAS_CONFIG_FLAG=true
        break
    fi
done

HAS_LOG_LEVEL_FLAG=false

for i in "$@" ; do
    if [[ "${i:0:11}" == "--log-level" ]] ; then
        HAS_LOG_LEVEL_FLAG=true
        break
    fi
done

if [ "$HAS_CONFIG_FLAG" = false -a "$HAS_LOG_LEVEL_FLAG" = true ] || [ $NUM_ARGS = 0 ] ; then
    set -- "$@" --config="$CONFIG_FILE"
fi

exec "$@"