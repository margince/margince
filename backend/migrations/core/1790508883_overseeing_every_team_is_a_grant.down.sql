-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The grant goes with the object it names: policy.Parse refuses a document
-- naming an object outside the closed set, so a leftover key would fail every
-- login. A hand-set grant on a CUSTOM role goes too, for the same reason.
UPDATE role SET permissions = permissions #- '{objects,team_oversight}'
    WHERE permissions ? 'objects' AND (permissions -> 'objects') ? 'team_oversight';
