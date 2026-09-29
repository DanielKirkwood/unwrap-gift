-- CreateRelationship does not sort its two arguments. The migration's
-- CHECK (phone_number_a < phone_number_b) will reject a call where
-- phone_number_a is not less than phone_number_b: the caller is
-- responsible for sorting the pair before calling this query.
-- name: CreateRelationship :one
INSERT INTO relationships (phone_number_a, phone_number_b) VALUES (?, ?) RETURNING *;

-- ListRelationshipsForPhoneNumber takes the same phone number twice
-- (PhoneNumberA and PhoneNumberB): it is not a two-number lookup. Passing
-- different values, or leaving one unset, silently returns only the
-- relationships where that phone number happens to be stored in the
-- matching column.
-- name: ListRelationshipsForPhoneNumber :many
SELECT * FROM relationships WHERE phone_number_a = ? OR phone_number_b = ? ORDER BY id;

-- name: DeleteRelationship :exec
DELETE FROM relationships WHERE id = ?;
