#!/bin/sh

# Install tools
apk --update add curl ca-certificates gcc git musl-dev && update-ca-certificates

# Install glide
curl https://glide.sh/get | sh

# Install dependencies
glide install --strip-vendor

# Build the service
cd service && go build -a -o kubernetes-vault

# Build the init container
cd ../init && go build -a -o kubernetes-vault-init

# Build the sample-app container
cd ../sample-app && go build -a -o sample-app