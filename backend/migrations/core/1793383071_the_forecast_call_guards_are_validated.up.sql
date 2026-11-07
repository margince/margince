-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the guards the previous file added. A file of its own
-- because VALIDATE takes a lighter lock than the ADD, and only a separate
-- transaction lets writers through while it scans. None can fail: the new
-- column is empty, and the kind list only grew.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution VALIDATE CONSTRAINT forecast_contribution_qualification_check;
ALTER TABLE assurance_exception VALIDATE CONSTRAINT assurance_exception_subject_kind_check;
