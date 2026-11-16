VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: run test build release
run:   ; go run ./cmd/homeservices
test:  ; go test ./...
build: ; CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/homeservices ./cmd/homeservices
release: ; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/homeservices-linux-amd64 ./cmd/homeservices
