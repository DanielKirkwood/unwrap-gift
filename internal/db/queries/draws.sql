-- name: CreateDraw :one
INSERT INTO draws (drawer_id, exchange_date, budget_amount) VALUES (?, ?, ?) RETURNING *;

-- name: GetDraw :one
SELECT * FROM draws WHERE id = ?;

-- name: ListDrawsByDrawer :many
SELECT * FROM draws WHERE drawer_id = ? ORDER BY exchange_date DESC;

-- name: UpdateDrawStatus :one
UPDATE draws SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DeleteDraw :exec
DELETE FROM draws WHERE id = ?;
