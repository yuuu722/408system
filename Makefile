.PHONY: test build dev check-independence
check-independence:
	node scripts/check-independence.mjs
test:
	node scripts/check-independence.mjs
	cd backend && go test ./...
	cd frontend && npm test
build:
	cd backend && go build ./cmd/api
	cd frontend && npm run build
dev:
	docker compose up --build
