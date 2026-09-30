-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A provider-defined category is the same container in every mailbox, so a
-- rule naming one may bind the whole installation.
--
-- The constraint this widens is right about what it was written for, and its
-- reasoning is kept verbatim below: a label somebody MADE lives in one mailbox
-- and names nothing in anybody else's, so a workspace-wide rule over it would
-- bind every other connection to an id that means nothing there.
--
-- Gmail's four categories are not that. They are provider-defined constants
-- with the same meaning in every Gmail mailbox that exists, and an installation
-- that wants none of its promotions captured is stating one fact about itself
-- rather than reaching into somebody's label list.
--
-- The exception is an ALLOW-LIST rather than a loosening: anything not named
-- here is still personal. Writing it into the CHECK rather than leaving code to
-- seed differently is what keeps the schema honest about which rules may bind
-- everybody — the alternative is a constraint that says one thing and a seeder
-- that quietly does another.
--
-- The list lives here AND in the Go that seeds the defaults, which is a drift
-- risk with the usual answer: a gate asserts the two name the same set.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_container_is_personal;

ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_container_is_personal
    CHECK (
        kind <> 'container'
        OR scope = 'user'
        -- Only the mailbox's owner has a label list, so a container rule is
        -- theirs. A workspace-wide one would name a container in one
        -- colleague's mailbox and bind everybody else's connections to an id
        -- that means nothing there — unless the container is one the PROVIDER
        -- defines, which means the same thing everywhere.
        OR value IN (
            'gmail:CATEGORY_PROMOTIONS',
            'gmail:CATEGORY_SOCIAL',
            'gmail:CATEGORY_UPDATES',
            'gmail:CATEGORY_FORUMS'
        )
    );
