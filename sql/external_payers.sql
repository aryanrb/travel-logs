-- name: UpsertExternalPayer :one
-- Reuses an existing payer record (same trip + name) to avoid duplicates (req 3.6).
INSERT INTO external_payers (trip_id, name, contact)
VALUES (@trip_id, @name, @contact)
ON CONFLICT (trip_id, name)
DO UPDATE SET contact = COALESCE(EXCLUDED.contact, external_payers.contact)
RETURNING *;

-- name: GetExternalPayer :one
SELECT * FROM external_payers WHERE id = @id;

-- name: ListExternalPayers :many
SELECT * FROM external_payers WHERE trip_id = @trip_id ORDER BY name;
