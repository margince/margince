-- Validate the three bounce checks on comms_outbound.
--
-- Together they say a bounce is recorded whole: a kind and a time arrive
-- together, and neither a reason nor a recipient stands without the time that
-- makes it a bounce. All three have bound new and changed rows since they were
-- added and none was ever checked against the rows already there.
--
-- One file because all three scan the SAME table: three files would take the
-- table's lock three times for one pass over one heap.
--
-- Its own file, apart from the migration that added them, so the scans run
-- under SHARE UPDATE EXCLUSIVE rather than under the ACCESS EXCLUSIVE that one
-- held until it committed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound VALIDATE CONSTRAINT comms_outbound_bounce_is_stated;
ALTER TABLE comms_outbound VALIDATE CONSTRAINT comms_outbound_bounce_kind_named;
ALTER TABLE comms_outbound VALIDATE CONSTRAINT comms_outbound_bounce_recipient_stated;
