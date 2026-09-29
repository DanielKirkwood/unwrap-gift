-- name: CreateMember :one
INSERT INTO members (drawer_id, full_name, phone_number) VALUES (?, ?, ?) RETURNING *;

-- name: GetMember :one
SELECT * FROM members WHERE id = ?;

-- name: ListMembersByDrawer :many
SELECT * FROM members WHERE drawer_id = ? ORDER BY id;

-- name: ListMembersByPhoneNumber :many
SELECT * FROM members WHERE phone_number = ? ORDER BY drawer_id;

-- name: UpdateMember :one
UPDATE members SET full_name = ?, phone_number = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DeleteMember :exec
DELETE FROM members WHERE id = ?;
