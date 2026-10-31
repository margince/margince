-- A uuid primary key defaults to uuidv7().
--
-- ids.NewV7 is the only id-minting function in the tree, and data-model 1.1
-- mandates v7 because ids minted by one process sort by creation order, which
-- keeps B-tree inserts append-mostly. Several tables key their pagination on
-- (created_at DESC, id DESC), and a random id in that tiebreak scatters
-- inserts across the whole index instead of appending.
--
-- 271 tables carried the default and twelve did not. Every live call site
-- already passes a v7, so this closes the gap rather than fixing a defect: a
-- new writer, an import path or a fixture could insert a v4 and nothing
-- refused it.
--
-- Adding a default is metadata only, so no table is rewritten and a caller
-- that passes its own id keeps working.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_owner_identity_withdrawn ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE commission_entry ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE comms_outbound ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE forecast_capture_status ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE mail_draft ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE report_definition ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE report_edition ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE report_execution ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE report_schedule ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE sales_target ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE scheduled_send ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE transcript_read ALTER COLUMN id SET DEFAULT uuidv7();
