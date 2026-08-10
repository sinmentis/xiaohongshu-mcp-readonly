.PHONY: build check fmt test test-race

build:
	go build -trimpath -o bin/xiaohongshu-mcp-readonly .

fmt:
	go fmt ./...

test:
	go test ./...

test-race:
	go test ./... -race

check: fmt
	go vet ./...
	go test ./...
	go test ./... -race
	go test -tags integration -run '^$$' ./...
