-- name: InsertTrip :one
INSERT INTO trips (
    owner_id, title, cover_image, description, start_date, end_date,
    start_location, trip_type, currency
) VALUES (
    @owner_id, @title, @cover_image, @description, @start_date, @end_date,
    @start_location, @trip_type, @currency
)
RETURNING *;

-- name: GetTrip :one
SELECT * FROM trips WHERE id = @id;

-- name: ListOwnedTrips :many
SELECT * FROM trips
WHERE owner_id = @owner_id
ORDER BY created_at DESC;

-- name: ListSharedTrips :many
SELECT sqlc.embed(trips), p.role
FROM trips
JOIN trip_participants p ON p.trip_id = trips.id
WHERE p.user_id = @user_id AND p.status = 'accepted'
ORDER BY trips.created_at DESC;

-- name: UpdateTrip :exec
UPDATE trips SET
    title = @title,
    cover_image = @cover_image,
    description = @description,
    start_date = @start_date,
    end_date = @end_date,
    start_location = @start_location,
    trip_type = @trip_type,
    currency = @currency,
    updated_at = now()
WHERE id = @id;

-- name: SetTripPublic :exec
UPDATE trips SET is_public = @is_public, updated_at = now() WHERE id = @id;

-- name: ArchiveTrip :exec
UPDATE trips SET archived_at = now(), updated_at = now() WHERE id = @id;

-- name: DeleteTrip :exec
DELETE FROM trips WHERE id = @id;

-- name: InsertDestination :one
INSERT INTO trip_destinations (trip_id, name, arrival_date, sort_order)
VALUES (@trip_id, @name, @arrival_date, @sort_order)
RETURNING *;

-- name: ListDestinations :many
SELECT * FROM trip_destinations WHERE trip_id = @trip_id ORDER BY sort_order, arrival_date;

-- name: DeleteDestination :exec
DELETE FROM trip_destinations WHERE id = @id AND trip_id = @trip_id;
