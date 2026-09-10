-- name: CreateMessage :one
INSERT INTO messages (
    user_id,
    body
) VALUES ($1, $2)
RETURNING *;
