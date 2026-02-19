# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.24 AS builder
WORKDIR /src

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
RUN set -eux; \
    GOARM=""; \
    if [ "$TARGETARCH" = "arm" ] && [ -n "$TARGETVARIANT" ]; then GOARM="${TARGETVARIANT#v}"; fi; \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" GOARM="${GOARM}" go build -trimpath -ldflags='-s -w -extldflags "-static"' -o /out/cloudflare-companion ./cmd/cloudflare-companion

FROM alpine:3.23 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
COPY --from=builder /out/cloudflare-companion /cloudflare-companion
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 65534:65534
ENTRYPOINT ["/cloudflare-companion"]
