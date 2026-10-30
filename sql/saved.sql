-- name: SaveTrip :exec
INSERT INTO saved_trips (user_id, trip_id) VALUES (@user_id, @trip_id)
ON CONFLICT (user_id, trip_id) DO NOTHING;

-- name: UnsaveTrip :exec
DELETE FROM saved_trips WHERE user_id = @user_id AND trip_id = @trip_id;

-- name: ListSavedTrips :many
-- Same row shape as ListPublicTrips, newest-saved first, public trips only.
SELECT
    sqlc.embed(t),
    u.name AS owner_name,
    COALESCE(array_agg(d.name) FILTER (WHERE d.name IS NOT NULL), '{}')::text[] AS destination_names
FROM saved_trips s
JOIN trips t ON t.id = s.trip_id
JOIN users u ON u.id = t.owner_id
LEFT JOIN trip_destinations d ON d.trip_id = t.id
WHERE s.user_id = @user_id AND t.is_public = true
GROUP BY t.id, u.name, s.created_at
ORDER BY s.created_at DESC;
