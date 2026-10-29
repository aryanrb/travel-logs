.PHONY: watch generate migrate rollback new tidy run test

include .env

tidy:
	go mod tidy

generate:
	go tool sqlc generate

migrate:
	POSTGRESQL_URL="$(POSTGRESQL_URL)" go tool tern migrate --migrations migrations/

rollback:
	POSTGRESQL_URL="$(POSTGRESQL_URL)" go tool tern migrate --migrations migrations/ --destination $(dest)

new:
	TERN_MIGRATIONS="migrations" go tool tern new $(name)

run:
	go run .

test:
	go test ./...

watch:
	@command -v air >/dev/null && air || echo "install air: go install github.com/air-verse/air@latest"
