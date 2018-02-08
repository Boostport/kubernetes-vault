FROM golang:1.9-alpine as builder

ARG SOURCE_COMMIT
ARG SOURCE_BRANCH

WORKDIR $GOPATH/src/github.com/Boostport/kubernetes-vault

COPY . .

RUN apk --update add gcc git musl-dev \
 && go get -u github.com/golang/dep/cmd/dep \
 && dep ensure \
 && go build -ldflags "-X 'main.commit=${SOURCE_COMMIT}' -X 'main.tag=${SOURCE_BRANCH}' -X 'main.buildDate=$(date -u)'" -o kubernetes-vault ./cmd/controller

FROM alpine:latest
LABEL maintainer="Francis Chuang <francis.chuang@boostport.com>"

RUN mkdir -p /kubernetes-vault

COPY --from=builder /go/src/github.com/Boostport/kubernetes-vault/kubernetes-vault /bin/kubernetes-vault
COPY --from=builder /go/src/github.com/Boostport/kubernetes-vault/cmd/controller/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["docker-entrypoint.sh"]