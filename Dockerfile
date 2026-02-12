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

FROM scratch
COPY --from=builder /out/cloudflare-companion /cloudflare-companion
USER 65532:65532
ENTRYPOINT ["/cloudflare-companion"]
