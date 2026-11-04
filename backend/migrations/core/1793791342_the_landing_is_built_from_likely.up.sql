-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The saved forward measure is spelled with the Likely names. Frozen snapshots
-- keep the old spelling, which the reader still parses.
SET LOCAL lock_timeout = '3s';

UPDATE setting SET value = to_jsonb('likely'::text)
 WHERE key = 'installation.forecast_forward_measure' AND value = to_jsonb('commit_evidence'::text);
UPDATE setting SET value = to_jsonb('qualified_likely'::text)
 WHERE key = 'installation.forecast_forward_measure' AND value = to_jsonb('qualified_commit_evidence'::text);
