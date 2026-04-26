SHELL := /bin/bash
GO    := go
PKG   := ./...
BIN   := openvcc
DIST  := dist

.PHONY: all build test test-race lint fmt vet tidy clean \
        compose-up compose-down compose-smoke \
        tofu-fmt tofu-validate tofu-check

all: lint test build

build:
	$(GO) build -o $(DIST)/$(BIN) ./cmd/openvcc

test:
	$(GO) test $(PKG)

test-race:
	$(GO) test -race -coverprofile=coverage.txt $(PKG)

lint:
	@command -v golangci-lint >/dev/null || { echo "install golangci-lint: https://golangci-lint.run"; exit 1; }
	golangci-lint run

fmt:
	$(GO) fmt $(PKG)

vet:
	$(GO) vet $(PKG)

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(DIST) coverage.txt coverage.html

compose-up:
	docker compose -f deploy/compose.yaml up -d --build

compose-down:
	docker compose -f deploy/compose.yaml down -v

compose-smoke:
	./deploy/smoke.sh

tofu-fmt:
	tofu fmt -check -recursive infra

tofu-validate:
	@for d in infra/modules/aws infra/modules/azure infra/examples/stateless infra/examples/stateful; do \
		echo "==> $$d"; (cd $$d && tofu init -backend=false >/dev/null && tofu validate) || exit 1; \
	done

tofu-check: tofu-fmt tofu-validate
