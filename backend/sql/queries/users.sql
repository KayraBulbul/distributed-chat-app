-- name: CreateUser :one
INSERT INTO users (
  username
) VALUES ($1)
RETURNING *;

-- name: FindUserByID :one
SELECT *
FROM users
WHERE user_id = $1;
