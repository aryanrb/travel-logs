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
UPDATE users SET name = @name, photo_url = @photo_url, bio = @bio WHERE id = @id;
