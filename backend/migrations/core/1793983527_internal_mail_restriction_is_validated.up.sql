SET lock_timeout = '3s';

ALTER TABLE activity VALIDATE CONSTRAINT activity_internal_mail_remains_restricted;
