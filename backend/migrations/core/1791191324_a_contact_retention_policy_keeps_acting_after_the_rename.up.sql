-- A retention policy written for people keeps acting now that they are contacts.
--
-- The record was renamed person -> contact in the schema, and the nightly
-- evaluator's selectors and the seeded defaults were renamed with it. Stored
-- policy rows were not. An installation seeded before the rename still holds
-- `person/no_consent_no_deal`, a scope the evaluator has no selector for, so it
-- logs that the policy was skipped and moves on, every night, while the
-- settings page lists the rule beside the others as if it acted.
--
-- Where an admin has since added the contact rule themselves, theirs is the
-- decision that stands and the stale row goes; otherwise the stale row is
-- carried over with its window, action and basis as they were set.
SET LOCAL lock_timeout = '3s';

DELETE FROM retention_policy p
 WHERE p.object_type = 'person'
   AND EXISTS (SELECT 1 FROM retention_policy c
                WHERE c.object_type = 'contact'
                  AND c.category IS NOT DISTINCT FROM p.category);

UPDATE retention_policy SET object_type = 'contact' WHERE object_type = 'person';
