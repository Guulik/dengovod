NODE_BIN := /opt/homebrew/opt/node@24/bin
export PATH := $(NODE_BIN):$(PATH)

.PHONY: dev dev-backend dev-frontend test build

dev:
	$(MAKE) -j2 dev-backend dev-frontend

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

test:
	cd backend && go test ./...
	cd frontend && npm test -- --run

build:
	cd backend && mkdir -p bin && go build -o bin/server ./cmd/server
	cd frontend && npm run build

