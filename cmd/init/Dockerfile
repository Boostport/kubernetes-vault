FROM golang:1.8-alpine as builder

WORKDIR $GOPATH/src/github.com/Boostport/kubernetes-vault

COPY . .

RUN apk --update add gcc git musl-dev \
 && go get -u github.com/Masterminds/glide \
 && glide install --strip-vendor \
 && go build -o kubernetes-vault-init ./cmd/init

FROM alpine:latest
LABEL maintainer Francis Chuang <francis.chuang@boostport.com>

COPY --from=builder /go/src/github.com/Boostport/kubernetes-vault/kubernetes-vault-init /kubernetes-vault-init

ENTRYPOINT ["/kubernetes-vault-init"]