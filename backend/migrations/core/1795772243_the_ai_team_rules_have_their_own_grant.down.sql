-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The system roles lose the grant the up gave them, so a later up grants it
-- again rather than finding a stale key and leaving it alone.
SET LOCAL lock_timeout = '3s';

UPDATE role SET permissions = permissions #- '{objects,ai_team_rules}'
    WHERE is_system AND permissions ? 'objects' AND (permissions -> 'objects') ? 'ai_team_rules';
