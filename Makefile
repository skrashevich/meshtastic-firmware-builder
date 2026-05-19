.PHONY: builder-image backend frontend backend-test frontend-test test

builder-image:
	docker build -t meshtastic-pio-builder:latest -f docker/platformio-builder/Dockerfile .

backend:
	cd backend && go run ./cmd/server

frontend:
	cd frontend && npm install && npm run dev

backend-test:
	cd backend && go test ./...

frontend-test:
	cd frontend && npm ci && npm run typecheck && npm test

test: backend-test frontend-test
