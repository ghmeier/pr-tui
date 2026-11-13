VERSION := $(shell git describe --tags --always --dirty)

install:
	go build -ldflags "-X main.version=$(VERSION)" -o $(HOME)/.local/bin/prs .

test:
	go test ./...
