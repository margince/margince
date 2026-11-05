-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The qualified forecast is retired: its switch has no reader, and a saved
-- qualified measure becomes the reading it restricted. Frozen snapshots keep
-- their old spelling, which the reader still parses.
SET LOCAL lock_timeout = '3s';

DELETE FROM setting WHERE key = 'assurance.qualified_forecast_enforced';
UPDATE setting SET value = to_jsonb('likely'::text)
 WHERE key = 'installation.forecast_forward_measure' AND value = to_jsonb('qualified_likely'::text);
UPDATE setting SET value = to_jsonb('weighted'::text)
 WHERE key = 'installation.forecast_forward_measure' AND value = to_jsonb('qualified_weighted'::text);
