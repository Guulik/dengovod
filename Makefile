NODE_BIN := /opt/homebrew/opt/node@24/bin
export PATH := $(NODE_BIN):$(PATH)

.PHONY: dev dev-backend dev-frontend dev-remote tailscale-setup tailscale-url test build

dev:
	$(MAKE) -j2 dev-backend dev-frontend

dev-remote: tailscale-setup
	@echo ""
	@echo "Remote URL: $$(./scripts/tailscale-url.sh)"
	@echo "Keep this Mac awake and leave make dev running."
	@echo ""
	$(MAKE) dev

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

tailscale-setup:
	@chmod +x scripts/tailscale-setup.sh scripts/tailscale-url.sh
	@./scripts/tailscale-setup.sh

tailscale-url:
	@chmod +x scripts/tailscale-url.sh
	@./scripts/tailscale-url.sh

test:
	cd backend && go test ./...
	cd frontend && npm test -- --run

build:
	cd backend && mkdir -p bin && go build -o bin/server ./cmd/server
	cd frontend && npm run build

