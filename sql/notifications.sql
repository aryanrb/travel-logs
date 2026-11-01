-- name: InsertNotification :one
INSERT INTO notifications (user_id, type, actor_id, trip_id, comment_id, rsvp_status)
VALUES (@user_id, @type, @actor_id, @trip_id, @comment_id, @rsvp_status)
RETURNING *;

-- name: ListNotifications :many
-- Newest first, keyset-paged on the optional `before` cursor.
SELECT n.*, a.name AS actor_name, a.photo_url AS actor_photo_url, t.title AS trip_title
FROM notifications n
LEFT JOIN users a ON a.id = n.actor_id
LEFT JOIN trips t ON t.id = n.trip_id
WHERE n.user_id = @user_id
  AND (sqlc.narg(before)::timestamptz IS NULL OR n.created_at < sqlc.narg(before)::timestamptz)
ORDER BY n.created_at DESC
LIMIT sqlc.arg(lim)::int;

-- name: CountUnreadNotifications :one
SELECT COUNT(*)::int FROM notifications WHERE user_id = @user_id AND read = false;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read = true WHERE user_id = @user_id AND read = false;

-- name: MarkNotificationsRead :exec
UPDATE notifications SET read = true WHERE user_id = @user_id AND id = ANY(@ids::uuid[]);
