SET LOCAL lock_timeout = '3s';
ALTER TABLE activity ADD COLUMN inbound_sender_authenticated boolean NOT NULL DEFAULT false;
