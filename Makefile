# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
DOCKER=docker
BINARY_NAME=vfeeg-backend
ORGANISATION=vfeeg-development
GLOBAL_ORG=eegfaktura
PLATFORM=ghcr.io

VERSION=v0.3.05

GOPATH := ${PWD}/..:${GOPATH}
export GOPATH

all: test build
build:
	$(GOBUILD) -o $(BINARY_NAME) -v -ldflags="-s -w"
test:
	$(GOTEST) -v ./...
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)

docker-clean:
	$(DOCKER) rmi ghcr.io/$(ORGANISATION)/$(BINARY_NAME):$(VERSION)

docker:
	$(DOCKER) build -t ghcr.io/$(ORGANISATION)/$(BINARY_NAME):$(VERSION) .
	$(DOCKER) image tag ghcr.io/$(ORGANISATION)/$(BINARY_NAME):$(VERSION) ghcr.io/$(GLOBAL_ORG)/$(BINARY_NAME):latest

push: docker
	$(DOCKER) push ghcr.io/$(ORGANISATION)/$(BINARY_NAME):$(VERSION)
	$(DOCKER) push ghcr.io/$(GLOBAL_ORG)/$(BINARY_NAME):latest

protoc:
	protoc --experimental_allow_proto3_optional=true --proto_path=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative ./proto/*.proto

altas-hash:
	atlas migrate hash --env local

altas-migrage:
	atlas migrate diff --env local
