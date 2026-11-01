-- name: InsertComment :one
INSERT INTO comments (trip_id, author_id, parent_id, body)
VALUES (@trip_id, @author_id, @parent_id, @body)
RETURNING *;

-- name: GetComment :one
SELECT * FROM comments WHERE id = @id;

-- name: UpdateComment :one
UPDATE comments SET body = @body, edited_at = now() WHERE id = @id RETURNING *;

-- name: DeleteComment :exec
DELETE FROM comments WHERE id = @id; -- replies cascade

-- name: ListComments :many
-- Oldest first (replies interleaved by created_at; the client groups by parent_id),
-- keyset-paged on the optional `after` cursor. Comments by @blocked ids are hidden.
SELECT c.id, c.trip_id, c.author_id, c.parent_id, c.body, c.created_at, c.edited_at,
       u.name AS author_name, u.photo_url AS author_photo_url
FROM comments c
JOIN users u ON u.id = c.author_id
WHERE c.trip_id = @trip_id
  AND c.author_id <> ALL (@blocked::uuid[])
  AND (sqlc.narg(after)::timestamptz IS NULL OR c.created_at > sqlc.narg(after)::timestamptz)
ORDER BY c.created_at, c.id
LIMIT sqlc.arg(lim)::int;
