-- name: CreateDraw :one
INSERT INTO draws (drawer_id, exchange_date, budget_amount) VALUES (?, ?, ?) RETURNING *;

-- name: GetDraw :one
SELECT * FROM draws WHERE id = ?;

-- name: ListDrawsByDrawer :many
SELECT * FROM draws WHERE drawer_id = ? ORDER BY exchange_date DESC;

-- name: UpdateDrawStatus :one
UPDATE draws SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- UpdateDraw only ever touches exchange_date/budget_amount, not status -
-- that stays UpdateDrawStatus's job. The app layer is responsible for
-- rejecting an update once a draw has left 'draft' status; this query has
-- no WHERE status = 'draft' guard, so a caller that skips that check would
-- silently succeed against an already-run draw.
-- name: UpdateDraw :one
UPDATE draws SET exchange_date = ?, budget_amount = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DeleteDraw :exec
DELETE FROM draws WHERE id = ?;

-- ListRecentCompletedDrawsByDrawer returns the drawer's most recent draws
-- that actually produced an assignment ('assigned' or 'notified'
-- status), excluding the draw currently being run, most-recent-first,
-- capped at limit. RunDraw uses this to build the history window; the
-- 'draft' filter keeps an unrelated in-progress draw for the same
-- drawer from ever being treated as history.
-- name: ListRecentCompletedDrawsByDrawer :many
SELECT * FROM draws
WHERE drawer_id = ? AND id != ? AND status != 'draft'
ORDER BY exchange_date DESC
LIMIT ?;
