-- 1793675407: a_host_may_choose_a_booking_alias.
SET LOCAL lock_timeout = '3s';

-- A friendly public address a host chooses for their booking page, such as
-- /book/alex. The random slug on booking_page keeps working beside it.
--
-- One row per alias ever claimed, live or retired. The unique key on alias
-- spans both, so a retired alias stays a tombstone: it never becomes another
-- host's page; only its own host may revive it (activities.claimBookingAlias).
-- Lowercase letters, digits and inner hyphens, 3 to 30 characters, so an
-- alias can never equal a 32-character random slug.
--
-- host_user_id and booking_page_id are SET NULL, never CASCADE: a deleted
-- member or page must not free the address for somebody else.
CREATE TABLE booking_alias (
    id uuid NOT NULL DEFAULT uuidv7(),
    alias text NOT NULL
        CONSTRAINT booking_alias_alias_shape CHECK (alias ~ '^[a-z0-9][a-z0-9-]{1,28}[a-z0-9]$'),
    host_user_id uuid REFERENCES app_user(id) ON DELETE SET NULL,
    booking_page_id uuid REFERENCES booking_page(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    retired_at timestamptz,
    CONSTRAINT booking_alias_pkey PRIMARY KEY (id),
    CONSTRAINT booking_alias_alias_key UNIQUE (alias)
);

-- A host has at most one live alias.
CREATE UNIQUE INDEX booking_alias_one_live_per_host ON booking_alias (host_user_id) WHERE retired_at IS NULL;
CREATE INDEX booking_alias_booking_page_idx ON booking_alias (booking_page_id);
