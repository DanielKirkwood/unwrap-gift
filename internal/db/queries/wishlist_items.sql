-- name: CreateWishlistItem :one
INSERT INTO wishlist_items (phone_number, item_name, size, url) VALUES (?, ?, ?, ?) RETURNING *;

-- name: ListWishlistItemsByPhoneNumber :many
SELECT * FROM wishlist_items WHERE phone_number = ? ORDER BY id;

-- name: UpdateWishlistItem :one
UPDATE wishlist_items SET item_name = ?, size = ?, url = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DeleteWishlistItem :exec
DELETE FROM wishlist_items WHERE id = ?;
