BIN          := bin/opps
PKG          := ./...
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS      := -X main.version=$(VERSION)

.PHONY: build test int e2e lint fmt clean

build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/opps

test:
	go test -race $(PKG)

int:
	go test -tags=integration -count=1 $(PKG)

e2e:
	go test -tags=e2e -count=1 $(PKG)

lint:
	@dirs=$$(go list -f '{{.Dir}}' $(PKG)); \
	out=$$(go tool gofumpt -l $$dirs); \
	if [ -n "$$out" ]; then \
		echo "gofumpt: needs formatting:"; echo "$$out"; \
		go tool gofumpt -d $$dirs; \
		exit 1; \
	fi; \
	out=$$(go tool goimports -l $$dirs); \
	if [ -n "$$out" ]; then \
		echo "goimports: needs formatting:"; echo "$$out"; \
		go tool goimports -d $$dirs; \
		exit 1; \
	fi
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not installed — see https://golangci-lint.run/docs/welcome/install/local/"; \
		exit 1; \
	}
	golangci-lint run

fmt:
	@dirs=$$(go list -f '{{.Dir}}' $(PKG)); \
	go tool goimports -w $$dirs; \
	go tool gofumpt -w $$dirs

clean:
	rm -rf bin coverage.out
