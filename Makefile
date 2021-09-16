.PHONY: help test clean cctools install
.DEFAULT_GOAL := help

help: ## Print this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Test all golang packages
	go test ./...

clean: ## Remove all compiled binaries
	rm -rf ./bin

cctools: clean ## Compile binary
	go build -o ./bin/cctools main.go 

install: cctools ## Compile binary and install
