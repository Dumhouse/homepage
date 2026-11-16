FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/homeservices ./cmd/homeservices

FROM alpine:3.22
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
ENTRYPOINT ["/usr/local/bin/homeservices"]
