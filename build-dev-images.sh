#!/bin/sh

VERSION=dev

# Build the binaries
docker run --rm -v "$PWD":/source/kubernetes-vault -w /source/kubernetes-vault golang:1.11-alpine ./build.sh

# Build the images
docker build -t boostport/kubernetes-vault:"$VERSION" -f cmd/controller/Dockerfile.dev cmd/controller/
docker build -t boostport/kubernetes-vault-init:"$VERSION" -f cmd/init/Dockerfile.dev cmd/init/
docker build -t boostport/kubernetes-vault-sample-app:"$VERSION" -f cmd/sample-app/Dockerfile.dev cmd/sample-app/

# Push images
docker push boostport/kubernetes-vault:"$VERSION"
docker push boostport/kubernetes-vault-init:"$VERSION"
docker push boostport/kubernetes-vault-sample-app:"$VERSION"
