-- +goose Up
-- Intentionally empty: this migration only proves the goose + sqlc pipeline
-- end to end. The first real domain table is introduced in Phase 7.
SELECT 1;

-- +goose Down
SELECT 1;
