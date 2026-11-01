-- name: InsertSettlement :one
INSERT INTO settlements (
    trip_id, from_user_id, from_external_id, to_user_id, to_external_id,
    amount, settled_date, status, notes, recorded_by, kind
) VALUES (
    @trip_id, @from_user_id, @from_external_id, @to_user_id, @to_external_id,
    @amount, @settled_date, @status, @notes, @recorded_by, @kind
)
RETURNING *;

-- name: GetSettlement :one
SELECT * FROM settlements WHERE id = @id;

-- name: ListSettlements :many
SELECT * FROM settlements WHERE trip_id = @trip_id ORDER BY settled_date DESC, created_at DESC;

-- name: DeleteSettlement :exec
DELETE FROM settlements WHERE id = @id;
