SET LOCAL lock_timeout = '3s';
DROP INDEX comms_outbound_party_admission;
ALTER TABLE comms_outbound DROP COLUMN party_admission_pending;
DROP TABLE capture_party_source_replay;
DROP TABLE capture_party_admission_cursor;
DROP TABLE capture_party_source;
