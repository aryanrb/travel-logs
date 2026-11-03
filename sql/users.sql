-- name: InsertUser :one
INSERT INTO users (email, password_hash, name)
VALUES (@email, @password_hash, @name)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = @email;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = @id;

-- name: GetUsersByIDs :many
SELECT id, name, email, photo_url FROM users WHERE id = ANY(@ids::uuid[]);

-- name: UpdateUserProfile :exec
-- PUT /me is a partial update: photo_url/bio change only when the field was sent
-- (@set_photo / @set_bio). An explicit "" arrives as NULL here and clears it; an
-- omitted field leaves the stored value untouched.
UPDATE users SET
    name = @name,
    photo_url = CASE WHEN @set_photo::boolean THEN @photo_url ELSE photo_url END,
    bio = CASE WHEN @set_bio::boolean THEN @bio ELSE bio END
WHERE id = @id;
