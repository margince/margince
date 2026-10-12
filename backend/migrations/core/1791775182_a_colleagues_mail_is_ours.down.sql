-- The re-read rows go back to inbound with the colleague as counterparty, as
-- capture filed them. Their cleared verdicts are not restored: the next owed and
-- reply passes judge them again.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity DROP CONSTRAINT activity_colleague_mail_is_outbound;
UPDATE activity
   SET direction = 'inbound',
       counterparty_email = (SELECT lower(email) FROM app_user WHERE id = sent_by_colleague)
 WHERE sent_by_colleague IS NOT NULL;
ALTER TABLE activity DROP COLUMN sent_by_colleague;
