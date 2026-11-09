-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

UPDATE setting SET value = to_jsonb('commit_evidence'::text)
 WHERE key = 'installation.forecast_forward_measure' AND value = to_jsonb('likely'::text);
UPDATE setting SET value = to_jsonb('qualified_commit_evidence'::text)
 WHERE key = 'installation.forecast_forward_measure' AND value = to_jsonb('qualified_likely'::text);
