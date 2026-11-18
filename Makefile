VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOBUILD := CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)"

.PHONY: run test build release checksums
run:   ; go run ./cmd/homeservices
test:  ; go test ./...
build: ; $(GOBUILD) -o bin/homeservices ./cmd/homeservices

# Linux binaries for x86-64 servers, 64-bit ARM (Raspberry Pi 4/5, Ampere)
# and 32-bit ARM (older Pis), plus SHA-256 checksums.
release:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o bin/homeservices-linux-amd64 ./cmd/homeservices
	GOOS=linux GOARCH=arm64 $(GOBUILD) -o bin/homeservices-linux-arm64 ./cmd/homeservices
	GOOS=linux GOARCH=arm GOARM=7 $(GOBUILD) -o bin/homeservices-linux-armv7 ./cmd/homeservices
	$(MAKE) checksums
checksums:
	cd bin && (command -v sha256sum >/dev/null && sha256sum homeservices-linux-* || shasum -a 256 homeservices-linux-*) | grep -v SHA256SUMS > SHA256SUMS
