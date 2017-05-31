#!/usr/bin/env bash

VERSION=dev

# Build the binaries
docker run --rm -v $PWD:/go/src/github.com/Boostport/kubernetes-vault -w /go/src/github.com/Boostport/kubernetes-vault golang:1.8-alpine ./build.sh

# Build the images
docker build -t boostport/kubernetes-vault:$VERSION -f controller/Dockerfile.dev controller/
docker build -t boostport/kubernetes-vault-init:$VERSION -f init/Dockerfile.dev init/
docker build -t boostport/kubernetes-vault-sample-app:$VERSION -f sample-app/Dockerfile.dev sample-app/

# Push images
docker push boostport/kubernetes-vault:$VERSION
docker push boostport/kubernetes-vault-init:$VERSION
docker push boostport/kubernetes-vault-sample-app:$VERSION