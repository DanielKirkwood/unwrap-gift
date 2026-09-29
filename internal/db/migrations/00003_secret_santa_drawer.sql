-- +goose Up
CREATE TABLE drawers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    organiser_kratos_identity_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    drawer_id INTEGER NOT NULL REFERENCES drawers (id) ON DELETE CASCADE,
    full_name TEXT NOT NULL,
    -- E.164 format (e.g. +447700900000), no spaces/dashes. Must match
    -- whatever Kratos's session phone trait returns for lookups in Phase
    -- 6 to work — see the plan's Notes, point 6.
    phone_number TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (drawer_id, phone_number)
);

CREATE INDEX idx_members_phone_number ON members (phone_number);

-- Global exclusion graph, reused across every drawer a person belongs to
-- (not scoped to a single drawer_id — see the PRD's Decisions Log). Keyed
-- by phone number (E.164, e.g. +447700900000 — see plan Notes, point 6)
-- rather than member_id/person_id, since phone_number is this system's
-- only stable, drawer-independent identifier for a person.
-- Canonically ordered (phone_number_a < phone_number_b) so (a, b) and
-- (b, a) can't both be inserted as distinct rows; callers MUST sort the
-- pair before INSERT — the CHECK below only rejects unsorted input, it
-- does not sort it for you.
CREATE TABLE relationships (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    phone_number_a TEXT NOT NULL,
    phone_number_b TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (phone_number_a < phone_number_b),
    UNIQUE (phone_number_a, phone_number_b)
);

CREATE INDEX idx_relationships_phone_number_a ON relationships (phone_number_a);
CREATE INDEX idx_relationships_phone_number_b ON relationships (phone_number_b);

CREATE TABLE draws (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    drawer_id INTEGER NOT NULL REFERENCES drawers (id) ON DELETE CASCADE,
    exchange_date TIMESTAMP NOT NULL,
    -- Minor currency units (e.g. pence). Single-currency assumption for
    -- v1 — see the plan's Notes for why no currency column exists yet.
    budget_amount INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'assigned', 'notified')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_draws_drawer_id ON draws (drawer_id);

CREATE TABLE assignments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    draw_id INTEGER NOT NULL REFERENCES draws (id) ON DELETE CASCADE,
    gifter_member_id INTEGER NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    giftee_member_id INTEGER NOT NULL REFERENCES members (id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (gifter_member_id != giftee_member_id),
    UNIQUE (draw_id, gifter_member_id),
    UNIQUE (draw_id, giftee_member_id)
);

-- Phone-number-keyed, not member_id/drawer_id-keyed: a wishlist is
-- intentionally global and persistent across drawers/years (PRD Decisions
-- Log). No FK to members/drawers is correct here, not an oversight.
CREATE TABLE wishlist_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    phone_number TEXT NOT NULL,
    item_name TEXT NOT NULL,
    size TEXT,
    url TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_wishlist_items_phone_number ON wishlist_items (phone_number);

-- +goose Down
DROP TABLE wishlist_items;
DROP TABLE assignments;
DROP TABLE draws;
DROP TABLE relationships;
DROP TABLE members;
DROP TABLE drawers;
