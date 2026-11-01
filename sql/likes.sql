-- name: Like :exec
INSERT INTO trip_likes (user_id, trip_id) VALUES (@user_id, @trip_id)
ON CONFLICT DO NOTHING;

-- name: Unlike :exec
DELETE FROM trip_likes WHERE user_id = @user_id AND trip_id = @trip_id;

-- name: IsLiked :one
SELECT EXISTS (SELECT 1 FROM trip_likes WHERE user_id = @user_id AND trip_id = @trip_id);

-- name: CountLikes :one
SELECT COUNT(*)::int FROM trip_likes WHERE trip_id = @trip_id;

-- name: CountSaves :one
SELECT COUNT(*)::int FROM saved_trips WHERE trip_id = @trip_id;

-- name: IsSaved :one
SELECT EXISTS (SELECT 1 FROM saved_trips WHERE user_id = @user_id AND trip_id = @trip_id);

-- name: LikedTripIDsFor :many
SELECT trip_id FROM trip_likes WHERE user_id = @user_id AND trip_id = ANY(@trip_ids::uuid[]);

-- name: SavedTripIDsFor :many
SELECT trip_id FROM saved_trips WHERE user_id = @user_id AND trip_id = ANY(@trip_ids::uuid[]);
