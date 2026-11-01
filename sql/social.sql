-- ─── Follows ──────────────────────────────────────────────────────────────────

-- name: Follow :exec
INSERT INTO follows (follower_id, followee_id) VALUES (@follower_id, @followee_id)
ON CONFLICT DO NOTHING;

-- name: Unfollow :exec
DELETE FROM follows WHERE follower_id = @follower_id AND followee_id = @followee_id;

-- name: IsFollowing :one
SELECT EXISTS (SELECT 1 FROM follows WHERE follower_id = @follower_id AND followee_id = @followee_id);

-- name: FollowedIDs :many
SELECT followee_id FROM follows WHERE follower_id = @follower_id;

-- name: CountFollowers :one
SELECT COUNT(*)::int FROM follows WHERE followee_id = @user_id;

-- name: CountFollowing :one
SELECT COUNT(*)::int FROM follows WHERE follower_id = @user_id;

-- ─── Friendships ──────────────────────────────────────────────────────────────

-- name: GetFriendshipPair :one
-- The row between two users in either direction, if any.
SELECT * FROM friendships
WHERE (requester_id = @a AND addressee_id = @b)
   OR (requester_id = @b AND addressee_id = @a);

-- name: InsertFriendRequest :exec
INSERT INTO friendships (requester_id, addressee_id, status)
VALUES (@requester_id, @addressee_id, 'pending')
ON CONFLICT DO NOTHING;

-- name: AcceptFriendRequest :exec
UPDATE friendships SET status = 'accepted'
WHERE requester_id = @requester_id AND addressee_id = @addressee_id;

-- name: DeleteFriendshipPair :exec
DELETE FROM friendships
WHERE (requester_id = @a AND addressee_id = @b)
   OR (requester_id = @b AND addressee_id = @a);

-- name: CountFriends :one
SELECT COUNT(*)::int FROM friendships
WHERE status = 'accepted' AND (requester_id = @user_id OR addressee_id = @user_id);

-- name: ListAcceptedFriends :many
SELECT u.id, u.name, u.photo_url
FROM friendships f
JOIN users u ON u.id = CASE WHEN f.requester_id = @user_id THEN f.addressee_id ELSE f.requester_id END
WHERE f.status = 'accepted' AND (f.requester_id = @user_id OR f.addressee_id = @user_id)
ORDER BY u.name;

-- name: ListIncomingRequests :many
SELECT u.id, u.name, u.photo_url
FROM friendships f
JOIN users u ON u.id = f.requester_id
WHERE f.addressee_id = @user_id AND f.status = 'pending'
ORDER BY u.name;

-- name: ListOutgoingRequests :many
SELECT u.id, u.name, u.photo_url
FROM friendships f
JOIN users u ON u.id = f.addressee_id
WHERE f.requester_id = @user_id AND f.status = 'pending'
ORDER BY u.name;

-- ─── Blocks ───────────────────────────────────────────────────────────────────

-- name: Block :exec
INSERT INTO blocks (blocker_id, blocked_id) VALUES (@blocker_id, @blocked_id)
ON CONFLICT DO NOTHING;

-- name: Unblock :exec
DELETE FROM blocks WHERE blocker_id = @blocker_id AND blocked_id = @blocked_id;

-- name: IsBlockedEitherWay :one
SELECT EXISTS (
    SELECT 1 FROM blocks
    WHERE (blocker_id = @a AND blocked_id = @b)
       OR (blocker_id = @b AND blocked_id = @a)
);

-- name: DidBlock :one
SELECT EXISTS (SELECT 1 FROM blocks WHERE blocker_id = @blocker_id AND blocked_id = @blocked_id);

-- name: ListBlocked :many
SELECT u.id, u.name, u.photo_url
FROM blocks b
JOIN users u ON u.id = b.blocked_id
WHERE b.blocker_id = @blocker_id
ORDER BY u.name;

-- name: BlockedUserIDs :many
-- Every user id blocked by @user_id or who blocks @user_id (either direction).
SELECT DISTINCT (CASE WHEN blocker_id = @user_id THEN blocked_id ELSE blocker_id END)::uuid AS id
FROM blocks
WHERE blocker_id = @user_id OR blocked_id = @user_id;

-- name: DeleteFollowsBothWays :exec
DELETE FROM follows
WHERE (follower_id = @a AND followee_id = @b)
   OR (follower_id = @b AND followee_id = @a);

-- ─── Reports ──────────────────────────────────────────────────────────────────

-- name: InsertReport :one
INSERT INTO reports (reporter_id, subject_type, subject_id, reason, details)
VALUES (@reporter_id, @subject_type, @subject_id, @reason, @details)
RETURNING *;

-- ─── Profile: owner's public trips (explore row shape) ────────────────────────

-- name: ListPublicTripsByOwner :many
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
WHERE t.is_public = true AND t.owner_id = @owner_id
GROUP BY t.id, u.name
ORDER BY t.created_at DESC;

-- name: CountPublicTripsByOwner :one
SELECT COUNT(*)::int FROM trips WHERE owner_id = @owner_id AND is_public = true;
