FROM golang:1.8-alpine as builder

WORKDIR $GOPATH/src/github.com/Boostport/kubernetes-vault

COPY . .

RUN apk --update add gcc git musl-dev \
 && go get -u github.com/Masterminds/glide \
 && glide install --strip-vendor \
 && go build -o kubernetes-vault ./cmd/controller

FROM alpine:latest
LABEL maintainer Francis Chuang <francis.chuang@boostport.com>

RUN mkdir -p /kubernetes-vault

COPY --from=builder /go/src/github.com/Boostport/kubernetes-vault/kubernetes-vault /bin/kubernetes-vault
COPY --from=builder /go/src/github.com/Boostport/kubernetes-vault/cmd/controller/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["docker-entrypoint.sh"]