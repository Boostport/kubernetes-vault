#!/bin/sh

# Install tools
apk --update add gcc git musl-dev

# Install dep
go get -u github.com/golang/dep/cmd/dep

# Install dependencies
dep ensure

# Build the service
go build -a -o cmd/controller/kubernetes-vault ./cmd/controller/

# Build the init container
go build -a -o cmd/init/kubernetes-vault-init ./cmd/init/

# Build the sample-app container
go build -a -o cmd/sample-app/sample-app ./cmd/sample-app/