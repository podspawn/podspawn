BINARY := podspawn
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS := -ldflags "-X github.com/podspawn/podspawn/cmd.Version=$(VERSION) -X github.com/podspawn/podspawn/cmd.Commit=$(COMMIT)"

.PHONY: build test lint clean install hooks

build:
	go build $(LDFLAGS) -o $(BINARY) .

test:
	go test -count=1 ./...

lint:
	go vet ./...
	golangci-lint run

clean:
	rm -f $(BINARY)

install: build
	cp $(BINARY) /usr/local/bin/

hooks:
	cp scripts/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
