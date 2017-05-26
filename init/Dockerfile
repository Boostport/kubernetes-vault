FROM alpine:3.5
MAINTAINER Francis Chuang <francis.chuang@boostport.com>

RUN apk --update add ca-certificates && update-ca-certificates

ADD kubernetes-vault-init /kubernetes-vault-init

ENTRYPOINT ["/kubernetes-vault-init"]
