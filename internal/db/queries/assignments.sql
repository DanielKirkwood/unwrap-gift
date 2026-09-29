-- name: CreateAssignment :one
INSERT INTO assignments (draw_id, gifter_member_id, giftee_member_id) VALUES (?, ?, ?) RETURNING *;

-- name: ListAssignmentsByDraw :many
SELECT * FROM assignments WHERE draw_id = ? ORDER BY id;
