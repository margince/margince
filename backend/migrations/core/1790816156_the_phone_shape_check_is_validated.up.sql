-- The second half of the phone shape check, and a file of its own because this
-- runner gives each migration ONE transaction: validating where the constraint
-- was added would run the scan under the ACCESS EXCLUSIVE that ALTER still holds.
-- VALIDATE takes SHARE UPDATE EXCLUSIVE, which readers and writers both pass.
--
-- The scan passes on what is already stored. Every path that writes this column
-- puts the value through values.ParsePhone first and stores what it returns —
-- contact_children normalises a submitted list in place before the row writer
-- sees it, and observednumbers, providerclaimfill, creatededupe,
-- contactprofilefieldwrite and profilefieldrestore each parse their own input.
-- The scan is run rather than left deferred because the planner does not trust an
-- unvalidated constraint, and it reads as provisional to the next reader; if an
-- installation does hold a row from before that was true, this fails the deploy
-- naming it, which is the loud failure worth having over silent drift.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_phone VALIDATE CONSTRAINT contact_phone_e164;
