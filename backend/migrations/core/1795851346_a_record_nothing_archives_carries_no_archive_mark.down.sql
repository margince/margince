-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field ADD COLUMN archived_at timestamptz;
ALTER TABLE voice_build ADD COLUMN archived_at timestamptz;
ALTER TABLE voice_profile_delta ADD COLUMN archived_at timestamptz;
ALTER TABLE voice_profile_version ADD COLUMN archived_at timestamptz;

DROP TRIGGER sales_initiative_owner_id_retirement_guard ON sales_initiative;
ALTER TABLE sales_initiative ADD COLUMN archived_at timestamptz;
CREATE TRIGGER sales_initiative_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON sales_initiative
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND NEW.archived_at IS NULL)
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
