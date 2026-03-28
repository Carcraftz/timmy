GO ?= go

.PHONY: tidy test build-cli build-backend docker-up docker-down

tidy:
	cd backend && $(GO) mod tidy
	cd cli && $(GO) mod tidy

test:
	cd backend && $(GO) test ./...
	cd cli && $(GO) test ./...

build-cli:
	mkdir -p dist
	cd cli && $(GO) build -o ../dist/timmy ./cmd/timmy

build-backend:
	mkdir -p dist
	cd backend && $(GO) build -o ../dist/timmyd ./cmd/timmyd

docker-up:
	docker compose -f infra/docker-compose.yml up -d

docker-down:
	docker compose -f infra/docker-compose.yml down
