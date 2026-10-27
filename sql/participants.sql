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
