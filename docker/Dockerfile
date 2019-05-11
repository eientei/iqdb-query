FROM golang as builder
WORKDIR /go/src/iqdb-query
COPY . .
RUN export GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 && \
    go mod download && \
    go install -tags netgo -ldflags '-w -extldflags "-static"' -v .
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/iqdb-query /
ENTRYPOINT ["/iqdb-query"]

