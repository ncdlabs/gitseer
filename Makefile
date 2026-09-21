.PHONY: deps frontend frontend-dev build test run stop restart tidy lint install

GO ?= go
BIN ?= bin/lens

deps:
	$(GO) mod tidy
	cd web && npm install

frontend:
	cd web && npm install && npm run build
	rm -rf internal/server/ui/dist
	mkdir -p internal/server/ui/dist
	cp -R web/dist/. internal/server/ui/dist/

frontend-dev:
	cd web && npm run start

build: frontend
	$(GO) build -o $(BIN) ./cmd/lens

build-go:
	$(GO) build -o $(BIN) ./cmd/lens

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
