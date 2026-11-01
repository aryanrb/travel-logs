-- name: GetRSVP :one
SELECT * FROM rsvps WHERE id = @id;

-- name: GetRSVPByTripUser :one
SELECT * FROM rsvps WHERE trip_id = @trip_id AND user_id = @user_id;

-- name: UpsertRSVP :one
INSERT INTO rsvps (trip_id, user_id, status, chosen, replied_at)
VALUES (@trip_id, @user_id, @status, @chosen, now())
ON CONFLICT (trip_id, user_id) DO UPDATE SET
    status = EXCLUDED.status,
    chosen = EXCLUDED.chosen,
    replied_at = now()
RETURNING *;

-- name: SetRSVPStatus :exec
UPDATE rsvps SET status = @status, replied_at = now() WHERE id = @id;

-- name: DeleteRSVP :exec
DELETE FROM rsvps WHERE id = @id;

-- name: ListRSVPs :many
SELECT r.id, r.user_id, u.name, u.photo_url, r.status, r.replied_at
FROM rsvps r
JOIN users u ON u.id = r.user_id
WHERE r.trip_id = @trip_id
ORDER BY r.replied_at;

-- name: CountGoing :one
SELECT COUNT(*)::int FROM rsvps WHERE trip_id = @trip_id AND status = 'going';

-- name: RSVPCounts :one
SELECT
    COUNT(*) FILTER (WHERE status = 'going')::int  AS going,
    COUNT(*) FILTER (WHERE status = 'maybe')::int  AS maybe
FROM rsvps WHERE trip_id = @trip_id;

-- name: EarliestWaitlisted :one
SELECT * FROM rsvps
WHERE trip_id = @trip_id AND status = 'waitlist'
ORDER BY replied_at, created_at
LIMIT 1;

-- name: GoingPeople :many
SELECT u.name, u.photo_url
FROM rsvps r
JOIN users u ON u.id = r.user_id
WHERE r.trip_id = @trip_id AND r.status = 'going'
ORDER BY r.replied_at
LIMIT sqlc.arg(lim)::int;
