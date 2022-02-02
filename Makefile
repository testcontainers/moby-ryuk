.PHONY: compile build build_all fmt lint test vet

SOURCE_FOLDER := .

BINARY_PATH ?= ./bin/moby-ryuk

GOARCH ?= amd64

ifdef GOOS
BINARY_PATH :=$(BINARY_PATH).$(GOOS)-$(GOARCH)
endif

default: build

build_all: vet fmt
	for GOOS in darwin linux windows; do \
		$(MAKE) compile GOOS=$$GOOS GOARCH=amd64 ; \
	done

compile:
	CGO_ENABLED=0 go build -v -ldflags '-s' -o $(BINARY_PATH) $(SOURCE_FOLDER)/

run:
	go run $(SOURCE_FOLDER)/main.go

build: vet fmt compile

fmt:
	go fmt $(SOURCE_FOLDER)/...

vet:
	go vet $(SOURCE_FOLDER)/...

lint:
	go lint $(SOURCE_FOLDER)/...

test:
	go test $(SOURCE_FOLDER)/...
