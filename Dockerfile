# Cross-compiles on the build machine, so multi-arch images (amd64, arm64,
# arm/v7) build quickly without emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT
RUN GOARM="${TARGETVARIANT#v}" CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/homeservices ./cmd/homeservices

FROM alpine:3.24
LABEL org.opencontainers.image.source="https://github.com/Dumhouse/homepage" \
      org.opencontainers.image.description="Self-hosted home-lab inventory with health checks and an MCP server" \
      org.opencontainers.image.licenses="AGPL-3.0-only"
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 -h /data homeservices
WORKDIR /data
COPY --from=build /out/homeservices /usr/local/bin/homeservices
USER homeservices
VOLUME ["/data"]
EXPOSE 8080
ENV HS_ADDR=:8080 \
    HS_DATA=/data \
    HS_AUTH=none
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/homeservices"]
