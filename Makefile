.PHONY: build check fmt setup test test-race

SITE ?= rednote
AGENT ?= none

build:
	go build -trimpath -o bin/xiaohongshu-mcp-readonly .

fmt:
	go fmt ./...

test:
	go test ./...

test-race:
	go test ./... -race

setup:
	./scripts/setup-local --site "$(SITE)" --agent "$(AGENT)"

check: fmt
	go vet ./...
	go test ./...
	go test ./... -race
	go test -tags integration -run '^$$' ./...
