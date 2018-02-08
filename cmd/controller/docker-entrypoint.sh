#!/usr/bin/env sh

CONFIG_FILE=/kubernetes-vault/kubernetes-vault.yml

# Write the configuration if it was passed in using the KUBERNETES_VAULT_LOCAL_CONFIG environment variable
if [ -n "$KUBERNETES_VAULT_LOCAL_CONFIG" ]; then
    echo "$KUBERNETES_VAULT_LOCAL_CONFIG" > "$CONFIG_FILE"
fi

NUM_ARGS=$#

# If the user is trying to run kubernetes-vault directly with some or no
# arguments, then pass them to kubernetes-vault.
if [ "${1%%"${1#?}"}" = '-' ] || [ "$NUM_ARGS" = 0 ] || [ "$NUM_ARGS" = 1 -a "$1" = "version" ]; then
    set -- kubernetes-vault "$@"
fi

HAS_CONFIG_FLAG=false

for i in "$@" ; do
    if [ "${i%"${i#????????}"}" = "--config" ] ; then
        HAS_CONFIG_FLAG=true
        break
    fi
done

HAS_LOG_LEVEL_FLAG=false

for i in "$@" ; do
    if [ "${i%"${i#???????????}"}" = "--log-level" ] ; then
        HAS_LOG_LEVEL_FLAG=true
        break
    fi
done

if { [ "$HAS_CONFIG_FLAG" = false ] && [ "$HAS_LOG_LEVEL_FLAG" = true ] ;} || [ "$NUM_ARGS" = 0 ] ; then
    set -- "$@" --config="$CONFIG_FILE"
fi

exec "$@"
