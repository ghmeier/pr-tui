install:
	go build -o $(HOME)/.local/bin/prs .

test:
	go test ./...
