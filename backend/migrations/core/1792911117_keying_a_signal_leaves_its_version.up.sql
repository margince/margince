-- A signal's touch trigger leaves version and updated_at alone for an update
-- that changes only the entity keys and their marker, which is what the data
-- task that keys old signals writes. A lifecycle approval pins the signal
-- version it showed, so a bump would make it stale with nothing changed.
-- Every other update bumps them as before, an update that changes nothing
-- included. The WHEN reads the keys trigger's result, which runs first by name.
SET LOCAL lock_timeout = '3s';

DROP TRIGGER trg_signal_updated ON signal;
CREATE TRIGGER trg_signal_updated
	BEFORE UPDATE ON signal
	FOR EACH ROW
	WHEN ((OLD.entity_unkeyed, OLD.entity_deal_id, OLD.entity_company_id, OLD.entity_contact_id, OLD.entity_project_id)
	        IS NOT DISTINCT FROM
	      (NEW.entity_unkeyed, NEW.entity_deal_id, NEW.entity_company_id, NEW.entity_contact_id, NEW.entity_project_id)
	   OR (to_jsonb(OLD) - ARRAY['entity_unkeyed', 'entity_deal_id', 'entity_company_id', 'entity_contact_id', 'entity_project_id'])
	        IS DISTINCT FROM
	      (to_jsonb(NEW) - ARRAY['entity_unkeyed', 'entity_deal_id', 'entity_company_id', 'entity_contact_id', 'entity_project_id']))
	EXECUTE FUNCTION set_updated_at_bump_version();
