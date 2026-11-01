-- name: ListPublicTripsForExplore :many
-- Public discovery with the aggregates the explore endpoint needs. q matches
-- title/description/start_location/destination names (case-insensitive);
-- trip_type filters exactly. Empty string = no filter. The remaining filters
-- (days, destination, cost+currency conversion, following), sorting and paging
-- happen in Go so currency conversion and cost filters stay correct.
-- ponytail: returns the whole public set (frontend loads all today); add a SQL
-- keyset page here if the public corpus ever gets large.
SELECT
    sqlc.embed(t),
    u.name AS owner_name,
    COALESCE(array_agg(DISTINCT d.name) FILTER (WHERE d.name IS NOT NULL), '{}')::text[] AS destination_names,
    COALESCE((SELECT SUM(e.amount) FROM expenses e WHERE e.trip_id = t.id AND e.hide_from_public = false), 0)::bigint AS total_spent,
    (1 + (SELECT COUNT(*) FROM trip_participants p WHERE p.trip_id = t.id AND p.status = 'accepted' AND p.role = 'collaborator'))::int AS travellers,
    (SELECT COUNT(*) FROM saved_trips s WHERE s.trip_id = t.id)::int AS saves_count,
    (SELECT COUNT(*) FROM trip_likes l WHERE l.trip_id = t.id)::int AS likes_count
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
ORDER BY t.created_at DESC;

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
