NAME=cct
COMPILE_DIR=./bin
INSTALL_DIR=~/bin

.PHONY: help test clean build install uninstall
.DEFAULT_GOAL := help

help: ## Print this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Test all golang packages
	go test ./...

clean: ## Remove all compiled binaries
	rm -rf $(COMPILE_DIR)

build: clean ## Compile binary
	go build -o $(COMPILE_DIR)/$(NAME) main.go 

install: build ## Compile binary and install
	cp $(COMPILE_DIR)/$(NAME) $(INSTALL_DIR)

uninstall: ## Delete the installed binary
	rm $(INSTALL_DIR)/$(NAME)