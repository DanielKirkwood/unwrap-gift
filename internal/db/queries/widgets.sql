-- name: CreateWidget :one
INSERT INTO widgets (name) VALUES (?) RETURNING *;

-- name: GetWidget :one
SELECT * FROM widgets WHERE id = ?;

-- name: ListWidgets :many
SELECT * FROM widgets ORDER BY id;

-- name: UpdateWidget :one
UPDATE widgets SET name = ? WHERE id = ? RETURNING *;

-- name: DeleteWidget :exec
DELETE FROM widgets WHERE id = ?;
