FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /retry-proxy \
    ./cmd/retry-proxy/

FROM scratch
COPY --from=builder /retry-proxy /retry-proxy
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
EXPOSE 8080
ENTRYPOINT ["/retry-proxy"]
CMD ["config.yaml"]
