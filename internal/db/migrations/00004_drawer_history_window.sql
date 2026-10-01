-- +goose Up
-- Per-drawer repeat-assignment lookback window: RunDraw excludes gifter→
-- giftee pairs seen in this many of the drawer's most recent completed
-- draws before falling back to relaxing it (see internal/app/draws.go).
ALTER TABLE drawers ADD COLUMN history_window_draws INTEGER NOT NULL DEFAULT 2;

-- +goose Down
ALTER TABLE drawers DROP COLUMN history_window_draws;
