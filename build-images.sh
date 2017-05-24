#!/usr/bin/env bash

VERSION=0.4.5

# Build the binaries
docker run --rm -v $PWD:/go/src/github.com/Boostport/kubernetes-vault -w /go/src/github.com/Boostport/kubernetes-vault golang:1.8-alpine ./build.sh

# Build the images
docker build -t boostport/kubernetes-vault:$VERSION controller/
docker build -t boostport/kubernetes-vault-init:$VERSION init/
docker build -t boostport/kubernetes-vault-sample-app:$VERSION sample-app/

# Push images
docker push boostport/kubernetes-vault:$VERSION
docker push boostport/kubernetes-vault-init:$VERSION
docker push boostport/kubernetes-vault-sample-app:$VERSION