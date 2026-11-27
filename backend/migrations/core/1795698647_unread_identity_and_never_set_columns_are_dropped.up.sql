-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- No statement, trigger, export or erasure path read these columns, or the
-- only reader filtered on a value no writer ever set. A check or index that
-- goes with them named a dropped column.
SET LOCAL lock_timeout = '3s';

ALTER TABLE vault_secret DROP COLUMN IF EXISTS key_version;

ALTER TABLE federated_identity DROP COLUMN IF EXISTS email_at_link;

ALTER TABLE project_phase_history DROP COLUMN IF EXISTS changed_by;

ALTER TABLE partner DROP COLUMN IF EXISTS source;

ALTER TABLE report_edition DROP COLUMN IF EXISTS report_revision;

ALTER TABLE auth_token DROP COLUMN IF EXISTS purpose;

ALTER TABLE finance_connection DROP COLUMN IF EXISTS archived_at;

ALTER TABLE voice_learning_signal DROP COLUMN IF EXISTS final_text,
    DROP COLUMN IF EXISTS qualifies_as_source,
    DROP COLUMN IF EXISTS transformations;

-- The partial unique indexes on these two go with them; the keys the previous
-- migration built hold the same rows.
ALTER TABLE activity_review_template DROP COLUMN IF EXISTS archived_at;

ALTER TABLE linkedin_connection DROP COLUMN IF EXISTS provider_member_ref;
