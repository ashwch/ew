BIN_DIR := bin
VERSION ?= dev
EW_LDFLAGS := -X main.version=$(VERSION)

.PHONY: build
build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(EW_LDFLAGS)" -o $(BIN_DIR)/ew ./cmd/ew
	go build -o $(BIN_DIR)/_ew ./cmd/_ew

.PHONY: fmt
fmt:
	gofmt -w $(shell find . -name '*.go')

.PHONY: run
run: build
	./$(BIN_DIR)/ew

.PHONY: test
test:
	go test ./...
	go test ./cmd/_ew

.PHONY: vet
vet:
	go vet ./...
	go vet ./cmd/_ew

.PHONY: preflight
preflight:
	./scripts/preflight.sh $(VERSION)
