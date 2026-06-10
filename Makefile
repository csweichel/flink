CLI_BINARY := bin/flink
SERVER_BINARY := bin/flink-server
ADDR ?= :8080
DATA ?= ./data
STORAGE ?= file
BASE_HOST ?=
CLIENT_DIR := client
FRONTEND_DIR := server/frontend

.PHONY: build test run clean client-install client-test client-build frontend-install frontend-typecheck frontend-build

client-install:
	cd $(CLIENT_DIR) && npm ci

client-test: client-install
	cd $(CLIENT_DIR) && npm test

client-build: client-install
	cd $(CLIENT_DIR) && npm run build
	cp $(CLIENT_DIR)/dist/flink.global.js $(FRONTEND_DIR)/static/flink.js

frontend-install:
	cd $(FRONTEND_DIR) && npm ci

frontend-typecheck: frontend-install
	cd $(FRONTEND_DIR) && npm run typecheck

frontend-build: client-build frontend-typecheck
	cd $(FRONTEND_DIR) && npm run build

build: frontend-build
	go build -o $(CLI_BINARY) ./cli
	go build -o $(SERVER_BINARY) ./server

test: client-test frontend-build
	go test ./cli/... ./server/...

run: frontend-build
	go run ./server --addr $(ADDR) --data $(DATA) --storage $(STORAGE) $(if $(BASE_HOST),--base-host $(BASE_HOST),)

clean:
	rm -rf bin $(CLIENT_DIR)/dist
