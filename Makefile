MODULE=github.com/pjover/espigol
BIN=bin/espigol

.PHONY: build run tui server test fmt vet tidy sqlc-generate migrate-status

fmt:
	go fmt ./...

vet:
	go vet ./...

build: fmt
	mkdir -p bin
	go build -o $(BIN) ./cmd/espigol

run: build
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
