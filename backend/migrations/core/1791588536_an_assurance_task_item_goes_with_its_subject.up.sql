-- An assurance task item goes when the deal, signal, offer or contract it is
-- about goes.
--
-- subject_id names a row in one of four tables, and no foreign key can span
-- them. Each branch now has a key column of its own with ON DELETE CASCADE.
-- The pair stays the column readers use.
--
-- The table collects history, so the keys must not rewrite it. A stored
-- generated column would, under ACCESS EXCLUSIVE. These are plain columns,
-- which ALTER TABLE adds from the catalog alone, and a trigger fills them from
-- the pair. Every row already here reads subject_unkeyed = true through the
-- column's added-default value, so nothing is backfilled and those rows keep
-- NULL keys: they do not cascade. Every row written from here on is keyed.
--
-- The CHECKs bind a keyed row to its pair, so a writer that bypassed the
-- trigger is refused rather than mis-keyed. They are validated in a later
-- file, where the scan lets writers through; they hold for every row here
-- because those all read subject_unkeyed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_task_item
	ADD COLUMN subject_unkeyed     boolean NOT NULL DEFAULT true,
	ADD COLUMN subject_deal_id     uuid,
	ADD COLUMN subject_signal_id   uuid,
	ADD COLUMN subject_offer_id    uuid,
	ADD COLUMN subject_contract_id uuid;
ALTER TABLE assurance_task_item ALTER COLUMN subject_unkeyed SET DEFAULT false;

-- The trigger, not the writer, decides whether a row is keyed: an insert is
-- always keyed, and an update keeps what the row was.
CREATE FUNCTION trg_assurance_task_item_subject_keys() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	IF TG_OP = 'INSERT' THEN
		NEW.subject_unkeyed := false;
	ELSE
		NEW.subject_unkeyed := OLD.subject_unkeyed;
	END IF;
	IF NOT NEW.subject_unkeyed THEN
		NEW.subject_deal_id     := CASE WHEN NEW.subject_kind = 'deal'     THEN NEW.subject_id END;
		NEW.subject_signal_id   := CASE WHEN NEW.subject_kind = 'signal'   THEN NEW.subject_id END;
		NEW.subject_offer_id    := CASE WHEN NEW.subject_kind = 'offer'    THEN NEW.subject_id END;
		NEW.subject_contract_id := CASE WHEN NEW.subject_kind = 'contract' THEN NEW.subject_id END;
	END IF;
	RETURN NEW;
END $$;

CREATE TRIGGER trg_assurance_task_item_subject_keys
	BEFORE INSERT OR UPDATE ON assurance_task_item
	FOR EACH ROW EXECUTE FUNCTION trg_assurance_task_item_subject_keys();

ALTER TABLE assurance_task_item
	ADD CONSTRAINT assurance_task_item_subject_deal_id_fkey FOREIGN KEY (subject_deal_id)
		REFERENCES deal(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT assurance_task_item_subject_signal_id_fkey FOREIGN KEY (subject_signal_id)
		REFERENCES signal(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT assurance_task_item_subject_offer_id_fkey FOREIGN KEY (subject_offer_id)
		REFERENCES offer(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT assurance_task_item_subject_contract_id_fkey FOREIGN KEY (subject_contract_id)
		REFERENCES contract(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT assurance_task_item_subject_deal_id_bound
		CHECK (subject_unkeyed OR subject_deal_id IS NOT DISTINCT FROM
			CASE WHEN subject_kind = 'deal' THEN subject_id END) NOT VALID,
	ADD CONSTRAINT assurance_task_item_subject_signal_id_bound
		CHECK (subject_unkeyed OR subject_signal_id IS NOT DISTINCT FROM
			CASE WHEN subject_kind = 'signal' THEN subject_id END) NOT VALID,
	ADD CONSTRAINT assurance_task_item_subject_offer_id_bound
		CHECK (subject_unkeyed OR subject_offer_id IS NOT DISTINCT FROM
			CASE WHEN subject_kind = 'offer' THEN subject_id END) NOT VALID,
	ADD CONSTRAINT assurance_task_item_subject_contract_id_bound
		CHECK (subject_unkeyed OR subject_contract_id IS NOT DISTINCT FROM
			CASE WHEN subject_kind = 'contract' THEN subject_id END) NOT VALID,
	-- What refuses a subject kind with no key column of its own: a fifth kind
	-- added to the kind CHECK without a fifth column cannot be written.
	ADD CONSTRAINT assurance_task_item_subject_shape
		CHECK (subject_unkeyed OR num_nonnulls(
			subject_deal_id, subject_signal_id, subject_offer_id, subject_contract_id) = 1) NOT VALID;
