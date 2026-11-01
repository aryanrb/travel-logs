-- name: GetPool :one
SELECT * FROM trip_pools WHERE trip_id = @trip_id;

-- name: UpsertPool :one
INSERT INTO trip_pools (trip_id, collector_id, per_person, due_date)
VALUES (@trip_id, @collector_id, @per_person, @due_date)
ON CONFLICT (trip_id) DO UPDATE SET
    collector_id = EXCLUDED.collector_id,
    per_person = EXCLUDED.per_person,
    due_date = EXCLUDED.due_date,
    updated_at = now()
RETURNING *;

-- name: DeletePool :exec
DELETE FROM trip_pools WHERE trip_id = @trip_id;

-- name: PoolCollected :one
-- Sum of completed contributions into the pool.
SELECT COALESCE(SUM(amount), 0)::bigint
FROM settlements WHERE trip_id = @trip_id AND kind = 'contribution' AND status = 'completed';

-- name: PoolSpent :one
-- Sum of expenses paid from the pool.
SELECT COALESCE(SUM(amount), 0)::bigint
FROM expenses WHERE trip_id = @trip_id AND from_pool = true;

-- name: ConfirmSettlement :exec
UPDATE settlements SET status = 'completed' WHERE id = @id;
