-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A test run refused for its file keeps the refusal's field, code and values
-- beside its English sentence, so a screen words it in the reader's language.
-- A row from before this column keeps only the sentence, which still shows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run ADD COLUMN IF NOT EXISTS error_refusal jsonb;
