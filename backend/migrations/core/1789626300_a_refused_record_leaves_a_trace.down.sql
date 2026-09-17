-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded: DROP takes ACCESS EXCLUSIVE, and an open transaction reading this
-- table would otherwise queue every write to it behind this statement for as
-- long as this migration is willing to wait, which is forever.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS extension_ingest_refusal;
