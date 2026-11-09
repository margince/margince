-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The key added beside this file is validated here, under SHARE UPDATE
-- EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer VALIDATE CONSTRAINT offer_legal_entity_id_fkey;
