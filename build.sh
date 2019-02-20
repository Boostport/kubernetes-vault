#!/bin/sh

# Install tools
apk --update add gcc git musl-dev

# Build the service
go build -ldflags "-X 'main.commit=dev' -X 'main.tag=dev' -X 'main.buildDate=$(date -u)'" -a -o cmd/controller/kubernetes-vault ./cmd/controller/

# Build the init container
go build -ldflags "-X 'main.commit=dev' -X 'main.tag=dev' -X 'main.buildDate=$(date -u)'" -a -o cmd/init/kubernetes-vault-init ./cmd/init/

# Build the sample-app container
go build -ldflags "-X 'main.commit=dev' -X 'main.tag=dev' -X 'main.buildDate=$(date -u)'" -a -o cmd/sample-app/sample-app ./cmd/sample-app/