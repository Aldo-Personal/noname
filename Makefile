.PHONY: api gateway worker fmt check test build web infra-up infra-down smoke migrate integration
api:
	go run ./cmd/api
gateway:
	go run ./cmd/gateway
worker:
	go run ./cmd/worker
web:
	npm run dev
fmt:
	gofmt -w cmd internal
check:
	npm run format:check
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...
	npm run typecheck
	npm test
	npm run build
	node scripts/smoke.mjs
test:
	go test -race ./...
	npm test
build:
	mkdir -p bin
	go build -trimpath -o bin/api ./cmd/api
	go build -trimpath -o bin/gateway ./cmd/gateway
	go build -trimpath -o bin/worker ./cmd/worker
	npm run build
infra-up:
	docker compose up -d --wait
infra-down:
	docker compose down

smoke:
	npm run build -w @infra/sdk
	node scripts/smoke.mjs

migrate:
	go run ./cmd/migrate
integration:
	test -n "$$TEST_DATABASE_URL"
	go test -race -count=1 ./internal/access ./internal/ethereum ./internal/platform/database
