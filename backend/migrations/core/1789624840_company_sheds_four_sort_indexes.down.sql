-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- NULLS LAST is not redundant on the two descending pairs even though both
-- columns are NOT NULL: storekit.ListSort.OrderBy() always renders
-- `<field> <dir> NULLS LAST`, and Postgres matches an ORDER BY against an
-- index's DECLARED ordering. Without the clause the planner falls back to an
-- Incremental Sort over a backward scan.
CREATE INDEX idx_company_name_keyset ON company
    USING btree (display_name, created_at DESC, id DESC) WHERE (archived_at IS NULL);
CREATE INDEX idx_company_name_keyset_desc ON company
    USING btree (display_name DESC NULLS LAST, created_at DESC, id DESC) WHERE (archived_at IS NULL);
CREATE INDEX idx_company_updated_keyset ON company
    USING btree (updated_at DESC NULLS LAST, created_at DESC, id DESC) WHERE (archived_at IS NULL);
CREATE INDEX idx_company_last_activity_keyset ON company
    USING btree (last_activity_at DESC NULLS LAST, created_at DESC, id DESC) WHERE (archived_at IS NULL);
