-- name: ListPublicTrips :many
-- Public discovery. q matches title/description/start_location/destination names
-- (case-insensitive); trip_type filters exactly. Empty string = no filter.
SELECT
    sqlc.embed(t),
    u.name AS owner_name,
    COALESCE(array_agg(d.name) FILTER (WHERE d.name IS NOT NULL), '{}')::text[] AS destination_names
FROM trips t
JOIN users u ON u.id = t.owner_id
LEFT JOIN trip_destinations d ON d.trip_id = t.id
WHERE t.is_public = true
  AND (
    sqlc.arg(q)::text = ''
    OR t.title ILIKE '%' || sqlc.arg(q)::text || '%'
    OR t.description ILIKE '%' || sqlc.arg(q)::text || '%'
    OR t.start_location ILIKE '%' || sqlc.arg(q)::text || '%'
    OR EXISTS (
        SELECT 1 FROM trip_destinations dd
        WHERE dd.trip_id = t.id AND dd.name ILIKE '%' || sqlc.arg(q)::text || '%'
    )
  )
  AND (sqlc.arg(trip_type)::text = '' OR t.trip_type = sqlc.arg(trip_type)::text)
GROUP BY t.id, u.name
ORDER BY t.created_at DESC
LIMIT sqlc.arg(lim)::int;

-- name: GetPublicTrip :one
-- Returns the trip only if it is public — callers get NoRows (→ 404) otherwise,
-- never revealing whether a private trip exists.
SELECT sqlc.embed(t), u.name AS owner_name
FROM trips t
JOIN users u ON u.id = t.owner_id
WHERE t.id = @id AND t.is_public = true;

-- name: ListPublicPeople :many
-- Owner is prepended in Go; this returns accepted collaborators only (no
-- viewers, no external payers, no emails or ids).
SELECT u.name, p.role
FROM trip_participants p
JOIN users u ON u.id = p.user_id
WHERE p.trip_id = @trip_id AND p.status = 'accepted' AND p.role = 'collaborator'
ORDER BY u.name;

-- name: PublicTotalExpenses :one
SELECT COALESCE(SUM(amount), 0)::bigint
FROM expenses WHERE trip_id = @trip_id AND hide_from_public = false;

-- name: PublicCategoryBreakdown :many
SELECT COALESCE(category, 'uncategorized') AS category, SUM(amount)::bigint AS total
FROM expenses
WHERE trip_id = @trip_id AND hide_from_public = false
GROUP BY category;
