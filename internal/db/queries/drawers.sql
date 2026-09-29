-- name: CreateDrawer :one
INSERT INTO drawers (name, organiser_kratos_identity_id) VALUES (?, ?) RETURNING *;

-- name: GetDrawer :one
SELECT * FROM drawers WHERE id = ?;

-- name: ListDrawersByOrganiser :many
SELECT * FROM drawers WHERE organiser_kratos_identity_id = ? ORDER BY id;

-- name: UpdateDrawer :one
UPDATE drawers SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DeleteDrawer :exec
DELETE FROM drawers WHERE id = ?;
