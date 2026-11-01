-- name: InsertParticipant :one
INSERT INTO trip_participants (trip_id, user_id, invited_email, role, status, invite_token)
VALUES (@trip_id, @user_id, @invited_email, @role, @status, @invite_token)
RETURNING *;

-- name: GetParticipant :one
SELECT * FROM trip_participants WHERE id = @id;

-- name: GetParticipantByTripUser :one
SELECT * FROM trip_participants WHERE trip_id = @trip_id AND user_id = @user_id;

-- name: GetParticipantByToken :one
SELECT * FROM trip_participants WHERE invite_token = @invite_token;

-- name: ListParticipants :many
SELECT p.*, u.name AS user_name, u.email AS user_email
FROM trip_participants p
LEFT JOIN users u ON u.id = p.user_id
WHERE p.trip_id = @trip_id
ORDER BY p.created_at;

-- name: UpdateParticipantRole :exec
UPDATE trip_participants SET role = @role WHERE id = @id AND trip_id = @trip_id;

-- name: SetParticipantStatus :exec
UPDATE trip_participants SET status = @status WHERE id = @id AND trip_id = @trip_id;

-- name: AcceptInvite :one
UPDATE trip_participants
SET status = 'accepted', user_id = @user_id, accepted_at = now()
WHERE invite_token = @invite_token AND status = 'pending'
RETURNING *;

-- name: UpsertRSVPParticipant :exec
-- RSVP Going/Maybe grants accepted access with the given role, creating or
-- reactivating the (trip, user) participant row.
INSERT INTO trip_participants (trip_id, user_id, role, status, accepted_at)
VALUES (@trip_id, @user_id, @role, 'accepted', now())
ON CONFLICT (trip_id, user_id) WHERE user_id IS NOT NULL
DO UPDATE SET role = EXCLUDED.role, status = 'accepted', accepted_at = now();

-- name: RevokeParticipantByUser :exec
-- Revokes a user's access to a trip (RSVP changed away from Going / reply removed).
UPDATE trip_participants SET status = 'revoked' WHERE trip_id = @trip_id AND user_id = @user_id;
