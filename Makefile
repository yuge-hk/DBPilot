.PHONY: build frontend backend test lint clean

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo 0.1.0-dev)

build: frontend backend

frontend:
	cd web && npm ci && npm run build

backend:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/dbpilot/dbpilot/internal/version.Version=$(VERSION)" -o bin/dbpilot ./cmd/dbpilot

test:
	go test ./...
	cd web && npm run typecheck

lint:
	gofmt -l .
	go vet ./...

clean:
	rm -rf bin web/dist
