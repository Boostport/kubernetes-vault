#!/bin/sh

# Install tools
apk --update add gcc git musl-dev

# Install glide
go get -u github.com/Masterminds/glide

# Install dependencies
glide install --strip-vendor

# Build the service
cd controller && go build -a -o kubernetes-vault

# Build the init container
cd ../init && go build -a -o kubernetes-vault-init

# Build the sample-app container
cd ../sample-app && go build -a -o sample-app