-- A data-subject request records which rule set its deadline, and whether the
-- officer chose an earlier date than that rule gives.
--
-- due_rule is "gdpr" or "<jurisdiction> v<version>". It is NULL on rows filed
-- before deadlines were computed, and those are not backfilled: nothing on
-- them says which rule a typed date came from.
SET LOCAL lock_timeout = '3s';

ALTER TABLE data_subject_request
    ADD COLUMN due_rule text,
    ADD COLUMN due_overridden boolean NOT NULL DEFAULT false;
