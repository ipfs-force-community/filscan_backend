GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION:=commit_$(shell git log -1 --pretty=format:%h)_time_$(shell date +"%Y-%m-%d_%H:%M:%S")

ifeq ($(GOHOSTOS), windows)
	#the `find.exe` is different from `find` in bash/shell.
	#to see https://docs.microsoft.com/en-us/windows-server/administration/windows-commands/find.
	#changed to use git-bash.exe to run find cli or other cli friendly, caused of every developer has a Git.
	#Git_Bash= $(subst cmd\,bin\bash.exe,$(dir $(shell where git)))
	Git_Bash=$(subst \,/,$(subst cmd\,bin\bash.exe,$(dir $(shell where git | grep cmd))))
	INTERNAL_PROTO_FILES=$(shell $(Git_Bash) -c "find internal -name *.proto")
	API_PROTO_FILES=$(shell $(Git_Bash) -c "find api -name *.proto")
else
	INTERNAL_PROTO_FILES=$(shell find internal -name *.proto)
	API_PROTO_FILES=$(shell find api -name *.proto)
endif

install:
	pip3 install mkdocs-material

.PHONY: init
# init env
init:
	go install github.com/google/wire/cmd/wire@latest
	go install golang.org/x/tools/cmd/goimports@latest

dep:
	git submodule update --init --recursive

build-api:
	make generate
	mkdir -p bin/ && go build --tags=bundle -ldflags "-X main.Version=$(VERSION) -X main.Name=filscan-api" -o ./bin/api ./cmd/filscan

bundle:
	go run cmd/gen-opengate-bundle/main.go

build-syncer:
	make generate
	make bundle
	make build-abi-decoder
	mkdir -p bin/ && go build --tags=bundle -ldflags "-X main.Version=$(VERSION) -X main.Name=filscan-syncer" -o ./bin/syncer ./cmd/syncer
build-abi-decoder:
	mkdir -p bin/ && go build --tags=bundle -ldflags "-X main.Version=$(VERSION) -X main.Name=abi-decoder" -o ./bin/abi-decoder ./cmd/abi-decoder

build-monitor:
	make generate
	mkdir -p bin/ && go build --tags=bundle -ldflags "-X main.Version=$(VERSION) -X main.Name=filscan-monitor" -o ./bin/monitor ./cmd/monitor

build: build-syncer build-api build-abi-decoder build-monitor 
.PHONY: build

build-calib: build-calib-syncer build-calib-api build-calib-abi-decoder build-calib-monitor
.PHONY: build-calib

build-calib-api:
	make generate
	mkdir -p bin/ && go build --tags=bundle,calibnet -ldflags "-X main.Version=$(VERSION) -X main.Name=filscan-api" -o ./bin/api ./cmd/filscan

build-calib-syncer:
	make generate
	make bundle
	make build-abi-decoder
	mkdir -p bin/ && go build --tags=bundle,calibnet -ldflags "-X main.Version=$(VERSION) -X main.Name=filscan-syncer" -o ./bin/syncer ./cmd/syncer

build-calib-abi-decoder:
	mkdir -p bin/ && go build --tags=bundle,calibnet -ldflags "-X main.Version=$(VERSION) -X main.Name=abi-decoder" -o ./bin/abi-decoder ./cmd/abi-decoder

build-calib-monitor:
	make generate
	mkdir -p bin/ && go build --tags=bundle,calibnet -ldflags "-X main.Version=$(VERSION) -X main.Name=filscan-monitor" -o ./bin/monitor ./cmd/monitor

run-api:
	go run cmd/filscan/filscan.go cmd/filscan/wire_gen.go -c configs/local.toml

run-syncer: 
	go run cmd/syncer/syncer.go cmd/syncer/wire_gen.go -c configs/local.toml

run-abi-decoder:
	go run cmd/abi-decoder/abi-decoder.go -c configs/local.toml

run-doc:
	mkdocs serve -f docs/mkdocs.yml

clients:
	go run cmd/jsonrpc-gen/jsonrpc-gen.go
	goimports modules/fevm/api/proxy_gen.go > modules/fevm/api/proxy_gen.go2
	rm -rf modules/fevm/api/proxy_gen.go
	mv modules/fevm/api/proxy_gen.go2 modules/fevm/api/proxy_gen.go
contract:
	go build -buildmode=plugin -o ./bin/contract.so ./cmd/contract/impl/contract_impl.go

.PHONY: generate
# generate
generate:
	go mod tidy
	go get github.com/google/wire/cmd/wire@latest
	go generate ./...

.PHONY: all
# generate all
all:
	make generate

# show help
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help

solc-select:
	pip3 install solc-select
	solc-select install all
	solc-select use latest

redis:
	docker pull redis
	docker run -d -p 6380:6379 -it --name="filscan-redis"  redis:7.0