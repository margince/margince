-- Dropping the marker table only. The emails the pass claimed stay outbound:
-- that is the correct reading of them, and the claim wrote its own audit trail.

SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS activity_own_sent_mail_repair;
