-- name: InsertTransport :one
INSERT INTO transportation (
    trip_id, entry_id, expense_id, type, start_point, destination,
    depart_at, distance_km, duration_min, cost_amount, notes, created_by,
    start_lat, start_lng, destination_lat, destination_lng
) VALUES (
    @trip_id, @entry_id, @expense_id, @type, @start_point, @destination,
    @depart_at, @distance_km, @duration_min, @cost_amount, @notes, @created_by,
    @start_lat, @start_lng, @destination_lat, @destination_lng
)
RETURNING *;

-- name: GetTransport :one
SELECT * FROM transportation WHERE id = @id;

-- name: ListTransport :many
SELECT * FROM transportation WHERE trip_id = @trip_id ORDER BY depart_at NULLS LAST, created_at;

-- name: UpdateTransport :exec
UPDATE transportation SET
    entry_id = @entry_id,
    expense_id = @expense_id,
    type = @type,
    start_point = @start_point,
    destination = @destination,
    depart_at = @depart_at,
    distance_km = @distance_km,
    duration_min = @duration_min,
    cost_amount = @cost_amount,
    notes = @notes,
    start_lat = @start_lat,
    start_lng = @start_lng,
    destination_lat = @destination_lat,
    destination_lng = @destination_lng
WHERE id = @id;

-- name: DeleteTransport :exec
DELETE FROM transportation WHERE id = @id;

-- name: TransportBreakdown :many
SELECT type, COUNT(*) AS count, COALESCE(SUM(distance_km), 0)::double precision AS total_distance_km
FROM transportation
WHERE trip_id = @trip_id
GROUP BY type;
