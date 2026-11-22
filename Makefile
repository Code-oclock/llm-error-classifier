.PHONY: fmt fmt-check lint test check build eval hooks

fmt:
	golangci-lint fmt

fmt-check:
	golangci-lint fmt --diff

lint:
	golangci-lint run

test:
	go test -race -count=1 ./...

check: fmt-check lint test

build:
	go build -o bin/classifier ./cmd/classifier
	go build -o bin/evaluate ./cmd/evaluate

eval:
	go run ./cmd/evaluate -url http://localhost:8080 -dataset eval/dataset.jsonl

hooks:
	git config core.hooksPath .githooks
