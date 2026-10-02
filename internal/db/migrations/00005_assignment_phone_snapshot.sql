-- +goose Up
-- SQLite can't ALTER a column's nullability or a foreign key's ON DELETE
-- action in place, so this rebuilds the assignments table instead of
-- altering it: adds gifter_phone_number/giftee_phone_number snapshot
-- columns (backfilled from members as part of the rebuild's copy step),
-- and changes gifter_member_id/giftee_member_id's FK from ON DELETE
-- CASCADE to ON DELETE SET NULL.
--
-- Deleting a member used to cascade-delete every assignment row
-- involving them, permanently erasing real pairing history -- including
-- the OTHER member's side of the pairing -- so if that phone number
-- later rejoined the drawer (a new members row, a new surrogate id), the
-- "last N draws" repeat-exclusion history had silently lost real data
-- and could no longer exclude a repeat (codebase-review-2026-10-02.md,
-- "Fix before you run the real draws" #7 / issue #23). An assignment row
-- is now identified durably by its phone-number snapshot, which survives
-- that member row's deletion; gifter_member_id/giftee_member_id becoming
-- NULL no longer erases the row, just the now-stale direct FK link. See
-- internal/app/draws.go's buildHistory, which resolves history by phone
-- number (not member_id) for exactly this reason.
--
-- Drawer deletion is intentionally NOT changed here: draws (and
-- therefore assignments) still cascade-delete when a whole drawer is
-- deleted, since that's a deliberate "tear down this recurring group"
-- action with no future draw left in that drawer to protect against
-- repeats for.
CREATE TABLE assignments_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    draw_id INTEGER NOT NULL REFERENCES draws (id) ON DELETE CASCADE,
    gifter_member_id INTEGER REFERENCES members (id) ON DELETE SET NULL,
    giftee_member_id INTEGER REFERENCES members (id) ON DELETE SET NULL,
    gifter_phone_number TEXT NOT NULL,
    giftee_phone_number TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        gifter_member_id IS NULL OR giftee_member_id IS NULL OR gifter_member_id != giftee_member_id
    ),
    UNIQUE (draw_id, gifter_member_id),
    UNIQUE (draw_id, giftee_member_id)
);

INSERT INTO assignments_new (
    id, draw_id, gifter_member_id, giftee_member_id,
    gifter_phone_number, giftee_phone_number, created_at
)
SELECT
    a.id, a.draw_id, a.gifter_member_id, a.giftee_member_id,
    gifter.phone_number, giftee.phone_number, a.created_at
FROM assignments a
JOIN members gifter ON gifter.id = a.gifter_member_id
JOIN members giftee ON giftee.id = a.giftee_member_id;

DROP TABLE assignments;
ALTER TABLE assignments_new RENAME TO assignments;

-- +goose Down
-- Lossy: any row whose gifter_member_id/giftee_member_id is NULL (the
-- member was deleted after this migration ran) can't be represented by
-- the old NOT NULL/CASCADE schema and is dropped.
CREATE TABLE assignments_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    draw_id INTEGER NOT NULL REFERENCES draws (id) ON DELETE CASCADE,
    gifter_member_id INTEGER NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    giftee_member_id INTEGER NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (gifter_member_id != giftee_member_id),
    UNIQUE (draw_id, gifter_member_id),
    UNIQUE (draw_id, giftee_member_id)
);

INSERT INTO assignments_old (id, draw_id, gifter_member_id, giftee_member_id, created_at)
SELECT id, draw_id, gifter_member_id, giftee_member_id, created_at
FROM assignments
WHERE gifter_member_id IS NOT NULL AND giftee_member_id IS NOT NULL;

DROP TABLE assignments;
ALTER TABLE assignments_old RENAME TO assignments;
