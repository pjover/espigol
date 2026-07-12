MODULE=github.com/pjover/espigol
BIN=bin/espigol
VERSION:=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS=-s -w -X main.version=$(VERSION)
DIST_TARGETS=linux-amd64 linux-arm64 darwin-arm64

.PHONY: build run tui server test fmt vet tidy sqlc-generate migrate-status \
	dist $(addprefix dist-,$(DIST_TARGETS)) clean

fmt:
	go fmt ./...

vet:
	go vet ./...

build: fmt
	mkdir -p bin
	go build -o $(BIN) ./cmd/espigol

run: build vet
	./$(BIN) $(ARGS)

tui: build
	./$(BIN)

server: build
	./$(BIN) --server

test:
	go test ./...

tidy:
	go mod tidy

sqlc-generate:
	go tool sqlc generate

migrate-status:
	@echo "migrations are applied automatically on Open; see db/migrations/"

# dist cross-compiles the deployable binary for every target in DIST_TARGETS,
# each as dist/<os>-<arch>/espigol, plus a combined SHA256SUMS.
dist: $(addprefix dist-,$(DIST_TARGETS))
	cd dist && sha256sum $(addsuffix /espigol,$(DIST_TARGETS)) > SHA256SUMS

dist-linux-amd64:
	mkdir -p dist/linux-amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/linux-amd64/espigol ./cmd/espigol

dist-linux-arm64:
	mkdir -p dist/linux-arm64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/linux-arm64/espigol ./cmd/espigol

dist-darwin-arm64:
	mkdir -p dist/darwin-arm64
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/darwin-arm64/espigol ./cmd/espigol

clean:
	rm -rf bin dist
