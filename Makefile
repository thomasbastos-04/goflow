.PHONY: up down logs test tidy run-api run-worker

up:
	cp -n .env.example .env 2>/dev/null || true
	docker compose up --build

down:
	docker compose down

reset:
	docker compose down -v

logs:
	docker compose logs -f api worker outbox

test:
	go test ./... -race -cover

tidy:
	go mod tidy

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker
