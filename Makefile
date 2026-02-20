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

.PHONY: tap-publish
tap-publish:
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "dev" ]; then echo "set VERSION=vX.Y.Z"; exit 1; fi
	@if [ -z "$(TAP_DIR)" ]; then echo "set TAP_DIR=/path/to/homebrew-tap"; exit 1; fi
	./scripts/publish_tap_formula.sh $(VERSION) $(TAP_DIR) $(CHANNEL)
