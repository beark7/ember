# Ember build targets. See AGENTS.md §3.

GO        ?= go
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG        = github.com/beark7/ember/internal/version
LDFLAGS    = -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)
BIN        = bin

.PHONY: all build test lint ui testdata e2e-mock e2e-tiny golden-update clean

all: build test lint

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/ ./cmd/...

test:
	$(GO) test ./... -race -count=1

lint:
	golangci-lint run ./...

ui:
	@echo "make ui: web/ not created yet (phase 6); backend builds with -tags noui"

testdata:
	@echo "make testdata: test model download arrives with T-010"

e2e-mock:
	@echo "make e2e-mock: arrives with T-014"

e2e-tiny:
	@echo "make e2e-tiny: arrives in phase 2"

golden-update:
	$(GO) test ./... -run 'Golden' -update

clean:
	rm -rf $(BIN)
