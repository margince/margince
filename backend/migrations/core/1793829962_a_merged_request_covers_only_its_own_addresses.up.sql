-- A request a contact made covers the addresses of the record it was made on.
-- A merge copies those addresses here, so after a wrong merge one person's
-- request does not authorize mail to the other person's addresses. NULL is a
-- request recorded on this record, which covers every address it holds.
SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_qualifying_event ADD COLUMN recorded_for_addresses text[];
ALTER TABLE contact_acquisition_evidence ADD COLUMN recorded_for_addresses text[];
