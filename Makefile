.PHONY: builder-image backend frontend backend-test frontend-test test

builder-image:
	docker build -t meshtastic-pio-builder:latest -f docker/platformio-builder/Dockerfile .

backend:
	cd backend && go run ./cmd/server

frontend:
	cd frontend && bun install && bun run dev

backend-test:
	cd backend && go test ./...

frontend-test:
	cd frontend && bun install --frozen-lockfile && bun run typecheck && bun run test

test: backend-test frontend-test

.PHONY: compose-build
compose-build:
	APP_VERSION=$$(git describe --tags --always --dirty) \
	APP_COMMIT=$$(git rev-parse --short=12 HEAD) \
	docker compose build --pull
