.PHONY: deps frontend frontend-dev build test run stop restart tidy lint install sqlc

GO ?= go
BIN ?= bin/gitseer
SQLC ?= sqlc

deps:
	$(GO) mod tidy
	cd web && npm install

sqlc:
	$(SQLC) generate

frontend:
	cd web && npm install && npm run build
	rm -rf internal/server/ui/dist
	mkdir -p internal/server/ui/dist
	cp -R web/dist/. internal/server/ui/dist/

frontend-dev:
	cd web && npm run start

build: frontend
	$(GO) build -o $(BIN) ./cmd/gitseer

build-go:
	$(GO) build -o $(BIN) ./cmd/gitseer

test:
	$(GO) test ./...

run:
	npm run start

stop:
	npm run stop

restart:
	npm run restart

install:
	./scripts/install.sh $(INSTALL_ARGS)

tidy:
	$(GO) mod tidy
