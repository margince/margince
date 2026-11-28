-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment
    ADD COLUMN owner_unkeyed boolean NOT NULL DEFAULT true,
    ADD COLUMN owner_contact_id uuid,
    ADD COLUMN owner_company_id uuid,
    ADD COLUMN owner_deal_id uuid,
    ADD COLUMN owner_lead_id uuid,
    ADD COLUMN owner_project_id uuid,
    ADD COLUMN owner_activity_id uuid;
ALTER TABLE attachment ALTER COLUMN owner_unkeyed SET DEFAULT false;

ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check
    CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'project', 'activity')) NOT VALID;

ALTER TABLE attachment
    ADD CONSTRAINT attachment_owner_contact_id_fkey FOREIGN KEY (owner_contact_id) REFERENCES contact(id) ON DELETE CASCADE NOT VALID,
    ADD CONSTRAINT attachment_owner_contact_id_bound CHECK (owner_unkeyed OR owner_contact_id IS NOT DISTINCT FROM CASE WHEN entity_type = 'contact' THEN entity_id END) NOT VALID,
    ADD CONSTRAINT attachment_owner_company_id_fkey FOREIGN KEY (owner_company_id) REFERENCES company(id) ON DELETE CASCADE NOT VALID,
    ADD CONSTRAINT attachment_owner_company_id_bound CHECK (owner_unkeyed OR owner_company_id IS NOT DISTINCT FROM CASE WHEN entity_type = 'company' THEN entity_id END) NOT VALID,
    ADD CONSTRAINT attachment_owner_deal_id_fkey FOREIGN KEY (owner_deal_id) REFERENCES deal(id) ON DELETE CASCADE NOT VALID,
    ADD CONSTRAINT attachment_owner_deal_id_bound CHECK (owner_unkeyed OR owner_deal_id IS NOT DISTINCT FROM CASE WHEN entity_type = 'deal' THEN entity_id END) NOT VALID,
    ADD CONSTRAINT attachment_owner_lead_id_fkey FOREIGN KEY (owner_lead_id) REFERENCES lead(id) ON DELETE CASCADE NOT VALID,
    ADD CONSTRAINT attachment_owner_lead_id_bound CHECK (owner_unkeyed OR owner_lead_id IS NOT DISTINCT FROM CASE WHEN entity_type = 'lead' THEN entity_id END) NOT VALID,
    ADD CONSTRAINT attachment_owner_project_id_fkey FOREIGN KEY (owner_project_id) REFERENCES project(id) ON DELETE CASCADE NOT VALID,
    ADD CONSTRAINT attachment_owner_project_id_bound CHECK (owner_unkeyed OR owner_project_id IS NOT DISTINCT FROM CASE WHEN entity_type = 'project' THEN entity_id END) NOT VALID,
    ADD CONSTRAINT attachment_owner_activity_id_fkey FOREIGN KEY (owner_activity_id) REFERENCES activity(id) ON DELETE CASCADE NOT VALID,
    ADD CONSTRAINT attachment_owner_activity_id_bound CHECK (owner_unkeyed OR owner_activity_id IS NOT DISTINCT FROM CASE WHEN entity_type = 'activity' THEN entity_id END) NOT VALID,
    ADD CONSTRAINT attachment_owner_shape CHECK (owner_unkeyed OR num_nonnulls(owner_contact_id, owner_company_id, owner_deal_id, owner_lead_id, owner_project_id, owner_activity_id) = 1) NOT VALID;

CREATE FUNCTION trg_attachment_owner_keys() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        NEW.owner_unkeyed := false;
    ELSE
        NEW.owner_unkeyed := OLD.owner_unkeyed AND NEW.owner_unkeyed;
    END IF;
    IF NOT NEW.owner_unkeyed THEN
        NEW.owner_contact_id := CASE WHEN NEW.entity_type = 'contact' THEN NEW.entity_id END;
        NEW.owner_company_id := CASE WHEN NEW.entity_type = 'company' THEN NEW.entity_id END;
        NEW.owner_deal_id := CASE WHEN NEW.entity_type = 'deal' THEN NEW.entity_id END;
        NEW.owner_lead_id := CASE WHEN NEW.entity_type = 'lead' THEN NEW.entity_id END;
        NEW.owner_project_id := CASE WHEN NEW.entity_type = 'project' THEN NEW.entity_id END;
        NEW.owner_activity_id := CASE WHEN NEW.entity_type = 'activity' THEN NEW.entity_id END;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_attachment_owner_keys BEFORE INSERT OR UPDATE ON attachment
    FOR EACH ROW EXECUTE FUNCTION trg_attachment_owner_keys();

-- Filling keys does not change when the file was last edited.
DROP TRIGGER trg_attachment_updated ON attachment;
CREATE TRIGGER trg_attachment_updated BEFORE UPDATE ON attachment
    FOR EACH ROW WHEN (
        (OLD.owner_unkeyed, OLD.owner_contact_id, OLD.owner_company_id, OLD.owner_deal_id, OLD.owner_lead_id, OLD.owner_project_id, OLD.owner_activity_id)
        IS NOT DISTINCT FROM
        (NEW.owner_unkeyed, NEW.owner_contact_id, NEW.owner_company_id, NEW.owner_deal_id, NEW.owner_lead_id, NEW.owner_project_id, NEW.owner_activity_id)
        OR (to_jsonb(OLD) - ARRAY['owner_unkeyed', 'owner_contact_id', 'owner_company_id', 'owner_deal_id', 'owner_lead_id', 'owner_project_id', 'owner_activity_id'])
        IS DISTINCT FROM
        (to_jsonb(NEW) - ARRAY['owner_unkeyed', 'owner_contact_id', 'owner_company_id', 'owner_deal_id', 'owner_lead_id', 'owner_project_id', 'owner_activity_id']))
    EXECUTE FUNCTION set_updated_at();
