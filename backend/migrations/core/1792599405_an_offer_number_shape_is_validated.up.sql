-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The offer-number shape added beside this file is validated here.
--
-- Separate file, so the ACCESS EXCLUSIVE the ALTER took has been committed and
-- this scan runs under SHARE UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer VALIDATE CONSTRAINT offer_number_shape;
