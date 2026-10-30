-- name: InsertEvent :one
INSERT INTO trip_events (trip_id, actor_id, actor_name, action, subject_type, subject_id, data)
VALUES (@trip_id, @actor_id, @actor_name, @action, @subject_type, @subject_id, @data)
RETURNING *;

-- name: ListEvents :many
-- Newest first, keyset-paged on created_at via the optional `before` cursor.
SELECT * FROM trip_events
WHERE trip_id = @trip_id
  AND (sqlc.narg(before)::timestamptz IS NULL OR created_at < sqlc.narg(before)::timestamptz)
ORDER BY created_at DESC
LIMIT sqlc.arg(lim)::int;
