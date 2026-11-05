SET LOCAL lock_timeout = '3s';

-- Why the sweep left a payload alone, beside the stamp that says it looked.
--
-- parts_slimmed_at is written on seven paths and only one of them removes
-- bytes. Without this a row examined and found unslimmable reads as a slimmed
-- one does, and the oversized-original question has to be answered from that
-- difference.
ALTER TABLE raw_capture ADD COLUMN parts_slim_outcome text;

-- NOT VALID because raw_capture is written continuously and every existing row
-- is NULL, which the constraint already allows. The validation runs in the
-- migration beside this one.
ALTER TABLE raw_capture
  ADD CONSTRAINT raw_capture_parts_slim_outcome_known
  CHECK (parts_slim_outcome IS NULL OR parts_slim_outcome IN (
    'slimmed',
    'no_proved_parts',
    'unreadable_spelling',
    'unrewritable_original',
    'nothing_came_out',
    'unencodable_result',
    'no_bytes_saved'
  )) NOT VALID;
