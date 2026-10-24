.PHONY: all build test race bench install clean lint

all: build

build:
	@mkdir -p bin
	go build -o bin/koodi ./cmd/koodi
	go build -o bin/koodi-cli ./cmd/koodi-cli
	@echo "Built bin/koodi and bin/koodi-cli"

test:
	go test -v -count=1 ./...

race:
	go test -count=1 -race ./...

bench:
	go test -bench=. -benchmem ./test/...

install:
	go install ./cmd/koodi ./cmd/koodi-cli
	@echo "Installed koodi and koodi-cli to GOPATH/bin"

clean:
	rm -rf bin/ koodi koodi-cli
	@echo "Cleaned bin/"
