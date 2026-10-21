SET LOCAL lock_timeout = '3s';
ALTER TABLE activity DROP COLUMN inbound_sender_authenticated;
