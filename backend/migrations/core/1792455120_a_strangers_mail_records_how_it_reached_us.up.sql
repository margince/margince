-- Three facts capture reads off an inbound mail, for ranking a stranger's mail.
--
-- capture_import.bcc_only: this seat's mailbox is in neither To nor Cc. It is a
-- fact per import, because two seats can receive one message differently.
-- activity.foreign_link: a web link in the body leads to a site unrelated to
-- the sender, after known redirect wrappers are unwrapped.
-- activity.sender_was_stranger: before the mail arrived, the sender was no live
-- contact and the workspace had never written to them.
--
-- NULL means not judged: anything but inbound mail, a Bcc question that an
-- unreadable To or Cc leaves open, and mail captured before these columns. The
-- waiting queue reads NULL as no evidence.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_import ADD COLUMN bcc_only boolean;
ALTER TABLE activity ADD COLUMN foreign_link boolean;
ALTER TABLE activity ADD COLUMN sender_was_stranger boolean;
