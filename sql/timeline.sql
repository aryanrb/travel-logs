-- name: InsertEntry :one
INSERT INTO timeline_entries (
    trip_id, title, description, entry_date, entry_time, location, lat, lng, created_by, last_modified_by
) VALUES (
    @trip_id, @title, @description, @entry_date, @entry_time, @location, @lat, @lng, @created_by, @created_by
)
RETURNING *;

-- name: GetEntry :one
SELECT * FROM timeline_entries WHERE id = @id;

-- name: ListEntries :many
SELECT * FROM timeline_entries
WHERE trip_id = @trip_id
ORDER BY entry_date, entry_time NULLS FIRST, created_at;

-- name: UpdateEntry :exec
UPDATE timeline_entries SET
    title = @title,
    description = @description,
    entry_date = @entry_date,
    entry_time = @entry_time,
    location = @location,
    lat = @lat,
    lng = @lng,
    last_modified_by = @last_modified_by,
    updated_at = now()
WHERE id = @id;

-- name: DeleteEntry :exec
DELETE FROM timeline_entries WHERE id = @id;
