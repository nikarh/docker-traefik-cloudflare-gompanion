# syntax=docker/dockerfile:1.7

FROM golang:1.26 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w -extldflags "-static"' -o /out/cloudflare-companion ./cmd/cloudflare-companion

FROM scratch
COPY --from=builder /out/cloudflare-companion /cloudflare-companion
USER 65532:65532
ENTRYPOINT ["/cloudflare-companion"]
