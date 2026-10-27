# travel-service

Phase 1 backend for the Travel Logging & Community Platform — travel journal,
shared trips with owner/collaborator/viewer roles, and expense tracking with
flexible payers and settlement balances. Echo + pgx + sqlc + tern.

See **[ROUTES.md](./ROUTES.md)** for the full API contract (frontend reference).

## Setup

```bash
cp .env.example .env            # set POSTGRESQL_URL and JWT_SECRET
go mod tidy
make migrate                    # apply migrations/ via tern
make generate                   # regenerate internal/models/ from sql/ (only after editing SQL)
make run                        # or: go run .
```

Needs `tern` (`go install github.com/jackc/tern/v2@latest`) and a Postgres DB.

## Tests

```bash
make test
```

Covers the money-split reconciliation and the settlement balance math (the parts
that must never drift).

## Layout

```
sql/           sqlc query sources        -> make generate
migrations/    tern schema (001_setup.sql, one file for Phase 1)
internal/
  auth/        JWT + bcrypt + middleware
  models/      sqlc GENERATED (do not edit)
  services/    business logic + DB + permission checks (authz.go)
  handler/     thin: parse -> service -> respond
  routes/v1/   route wiring (main.go)
  types/       request/response DTOs
  utils/       apperror, responder, money (split math)
```

## Adding an endpoint

1. SQL in `sql/<entity>.sql` → `make generate`.
2. Service method in `internal/services/<entity>.go` (owns DB + validation +
   permission check, returns `*apperror.Error`).
3. Thin handler in `internal/handler/<entity>.go`.
4. Register the route in `internal/routes/v1/main.go`.

## Not in this phase

Community features (public discovery, follow, bookmark, likes, comments, tips,
moderation) are Phase 2. Publishing is present only as an `is_public` flag + a
toggle endpoint — there's no public read API yet. Photo/receipt upload stores a
URL the frontend provides. See the defaults list at the bottom of ROUTES.md.
