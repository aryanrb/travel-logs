-- name: InsertPhoto :one
INSERT INTO photos (trip_id, entry_id, url, caption, taken_date, created_by)
VALUES (@trip_id, @entry_id, @url, @caption, @taken_date, @created_by)
RETURNING *;

-- name: GetPhoto :one
SELECT * FROM photos WHERE id = @id;

-- name: ListTripPhotos :many
SELECT * FROM photos WHERE trip_id = @trip_id ORDER BY taken_date NULLS LAST, created_at;

-- name: ListEntryPhotos :many
SELECT * FROM photos WHERE entry_id = @entry_id ORDER BY created_at;

-- name: CountTripPhotos :one
SELECT COUNT(*) FROM photos WHERE trip_id = @trip_id;

-- name: DeletePhoto :exec
DELETE FROM photos WHERE id = @id;
