-- sender_is_machine: an inbound mail whose sender address mailidentity judges a
-- sending system (a file-share notice, a notification, a delivery report).
-- False for every other row, so mail of unknown provenance keeps counting.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity ADD COLUMN sender_is_machine boolean NOT NULL DEFAULT false;
