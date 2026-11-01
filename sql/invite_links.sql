-- name: GetInviteLinkByTrip :one
SELECT * FROM invite_links WHERE trip_id = @trip_id;

-- name: GetInviteLinkByToken :one
SELECT * FROM invite_links WHERE token = @token;

-- name: UpsertInviteLink :one
-- Creates the link, or updates settings on an existing one. token is set only on
-- insert (keeps the current token on update; use ResetInviteLinkToken to rotate).
INSERT INTO invite_links (trip_id, token, active, join_mode, spots, reply_by, role, message)
VALUES (@trip_id, @token, @active, @join_mode, @spots, @reply_by, @role, @message)
ON CONFLICT (trip_id) DO UPDATE SET
    active = EXCLUDED.active,
    join_mode = EXCLUDED.join_mode,
    spots = EXCLUDED.spots,
    reply_by = EXCLUDED.reply_by,
    role = EXCLUDED.role,
    message = EXCLUDED.message,
    updated_at = now()
RETURNING *;

-- name: ResetInviteLinkToken :one
UPDATE invite_links SET token = @token, updated_at = now() WHERE trip_id = @trip_id RETURNING *;
