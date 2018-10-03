FROM golang:1.11-alpine as builder

ARG SOURCE_COMMIT
ARG SOURCE_BRANCH

WORKDIR /source

COPY . .

RUN apk --update add gcc git musl-dev \
 && go build -ldflags "-X 'main.commit=${SOURCE_COMMIT}' -X 'main.tag=${SOURCE_BRANCH}' -X 'main.buildDate=$(date -u)'" -o sample-app ./cmd/sample-app

FROM alpine:latest
LABEL maintainer="Francis Chuang <francis.chuang@boostport.com>"

COPY --from=builder /source/sample-app /sample-app

ENTRYPOINT ["/sample-app"]