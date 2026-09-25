SET LOCAL lock_timeout = '5s';

-- A pass somebody asked for records who asked.
--
-- captured_by stays the system principal on every run, nightly or not: the pass
-- reads the whole pipeline as the system, and a run captured_by a manager would
-- attribute the exceptions it raises to them. Who ASKED is a different fact and
-- gets its own column. NULL is the nightly cadence, which nobody asked for.
ALTER TABLE assurance_run
	ADD COLUMN requested_by text;

ALTER TABLE assurance_run
	ADD CONSTRAINT assurance_run_requester_is_named
	CHECK (requested_by IS NULL OR length(btrim(requested_by)) > 0);
