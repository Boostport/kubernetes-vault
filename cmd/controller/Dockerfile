FROM golang:1.11-alpine as builder

ARG SOURCE_COMMIT
ARG SOURCE_BRANCH

WORKDIR /source

COPY . .

RUN apk --update add gcc git musl-dev \
 && go build -ldflags "-X 'main.commit=${SOURCE_COMMIT}' -X 'main.tag=${SOURCE_BRANCH}' -X 'main.buildDate=$(date -u)'" -o kubernetes-vault ./cmd/controller

FROM alpine:latest
LABEL maintainer="Francis Chuang <francis.chuang@boostport.com>"

RUN mkdir -p /kubernetes-vault

COPY --from=builder /source/kubernetes-vault /bin/kubernetes-vault
COPY --from=builder /source/cmd/controller/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["docker-entrypoint.sh"]