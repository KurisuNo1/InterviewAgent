.PHONY: infra-up infra-down dev build clean restart logs help

# ===== Docker Infrastructure =====

infra-up:
	@echo "Starting infrastructure (Redis + MySQL + Milvus)..."
	docker compose up -d redis mysql etcd minio milvus
	@echo "Waiting for services to be healthy..."
	@timeout 30 bash -c 'until docker compose ps redis | grep healthy; do sleep 2; done' || true
	@timeout 30 bash -c 'until docker compose ps mysql | grep healthy; do sleep 2; done' || true
	@timeout 60 bash -c 'until docker compose ps milvus | grep healthy; do sleep 5; done' || true
	@echo "Infrastructure ready."

infra-down:
	docker compose down -v

infra-restart: infra-down infra-up

# ===== Application =====

build:
	go build -o bin/server ./cmd/server
	go build -o bin/cli ./cmd/cli
	@echo "Build complete: bin/server, bin/cli"

dev:
	@echo "Starting InterviewAgent server..."
	go run ./cmd/server

app-up:
	docker compose up -d app

app-down:
	docker compose stop app

# ===== Full stack =====

up:
	docker compose up -d
	@echo "All services running. App at http://localhost:8080"

down:
	docker compose down

restart: down up

# ===== Utility =====

logs:
	docker compose logs -f --tail=100

logs-app:
	docker compose logs -f app

clean:
	rm -rf bin/ data/bleve_index/
	docker compose down -v

test:
	go test ./... -count=1 -timeout=60s

lint:
	go vet ./...

deps:
	go mod tidy
	go mod download

# ===== Quick start =====

quickstart: infra-up
	@echo "Infrastructure ready. Run 'make dev' to start the server."
	@echo "Or 'make up' to run everything in Docker."

help:
	@echo "Usage:"
	@echo "  make infra-up      Start Redis + MySQL + Milvus"
	@echo "  make infra-down    Stop and remove infrastructure"
	@echo "  make dev           Run server locally (go run)"
	@echo "  make build         Build binaries"
	@echo "  make up            Start everything in Docker"
	@echo "  make down          Stop everything"
	@echo "  make logs          Tail all logs"
	@echo "  make clean         Clean build artifacts and data"
	@echo "  make test          Run tests"
	@echo "  make quickstart    Start infra, then manually run dev"
