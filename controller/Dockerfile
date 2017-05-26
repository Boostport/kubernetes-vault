FROM alpine:3.5
MAINTAINER Francis Chuang <francis.chuang@boostport.com>

RUN apk --update add ca-certificates && update-ca-certificates

RUN mkdir -p /kubernetes-vault

ADD kubernetes-vault /bin/kubernetes-vault
ADD docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["docker-entrypoint.sh"]
