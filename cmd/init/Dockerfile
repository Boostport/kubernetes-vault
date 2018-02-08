FROM golang:1.9-alpine as builder

ARG SOURCE_COMMIT
ARG SOURCE_BRANCH

WORKDIR $GOPATH/src/github.com/Boostport/kubernetes-vault

COPY . .

RUN apk --update add gcc git musl-dev \
 && go get -u github.com/golang/dep/cmd/dep \
 && dep ensure \
 && go build -ldflags "-X 'main.commit=${SOURCE_COMMIT}' -X 'main.tag=${SOURCE_BRANCH}' -X 'main.buildDate=$(date -u)'" -o kubernetes-vault-init ./cmd/init

FROM alpine:latest
LABEL maintainer="Francis Chuang <francis.chuang@boostport.com>"

COPY --from=builder /go/src/github.com/Boostport/kubernetes-vault/kubernetes-vault-init /kubernetes-vault-init

ENTRYPOINT ["/kubernetes-vault-init"]