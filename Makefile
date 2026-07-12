ifeq ($(OS),Windows_NT)
    TAILSCALE_SETUP = powershell -NoProfile -ExecutionPolicy Bypass -File scripts/tailscale-setup.ps1
    TAILSCALE_URL = powershell -NoProfile -ExecutionPolicy Bypass -File scripts/tailscale-url.ps1
else
    NODE_BIN := /opt/homebrew/opt/node@24/bin
    export PATH := $(NODE_BIN):$(PATH)
    TAILSCALE_SETUP = ./scripts/tailscale-setup.sh
    TAILSCALE_URL = ./scripts/tailscale-url.sh
endif

.PHONY: dev dev-backend dev-frontend dev-remote tailscale-setup tailscale-url test build

dev:
	$(MAKE) -j2 dev-backend dev-frontend

dev-remote: tailscale-setup
	@echo ""
ifeq ($(OS),Windows_NT)
	@echo Remote URL:
	@$(TAILSCALE_URL)
	@echo Keep this PC awake and leave make dev running.
else
	@echo "Remote URL: $$($(TAILSCALE_URL))"
	@echo "Keep this Mac awake and leave make dev running."
endif
	@echo ""
	$(MAKE) dev

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm run dev

tailscale-setup:
ifeq ($(OS),Windows_NT)
	@$(TAILSCALE_SETUP)
else
	@chmod +x scripts/tailscale-setup.sh scripts/tailscale-url.sh
	@$(TAILSCALE_SETUP)
endif

tailscale-url:
ifeq ($(OS),Windows_NT)
	@$(TAILSCALE_URL)
else
	@chmod +x scripts/tailscale-url.sh
	@$(TAILSCALE_URL)
endif

test:
	cd backend && go test ./...
	cd frontend && npm test -- --run

build:
	cd backend && mkdir -p bin && go build -o bin/server ./cmd/server
	cd frontend && npm run build

