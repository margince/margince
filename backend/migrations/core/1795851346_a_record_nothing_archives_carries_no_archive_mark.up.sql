-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- Nothing sets these marks. A custom field is retired through its status, a
-- voice record is deleted with its profile, and a sales initiative has no
-- archive path. The API still serves archived_at on a custom field and the
-- voice records, always null.
ALTER TABLE custom_field DROP COLUMN archived_at;
ALTER TABLE voice_build DROP COLUMN archived_at;
ALTER TABLE voice_profile_delta DROP COLUMN archived_at;
ALTER TABLE voice_profile_version DROP COLUMN archived_at;

-- The owner guard read the mark, so it is made again without it.
DROP TRIGGER sales_initiative_owner_id_retirement_guard ON sales_initiative;
ALTER TABLE sales_initiative DROP COLUMN archived_at;
CREATE TRIGGER sales_initiative_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON sales_initiative
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND true)
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
