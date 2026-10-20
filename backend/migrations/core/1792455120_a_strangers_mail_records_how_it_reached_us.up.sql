-- Three facts capture reads off an inbound mail, for ranking a stranger's mail.
--
-- bcc_only: the capturing mailbox is in neither To nor Cc, and no To or Cc
-- address is on one of our own domains. The copy came by Bcc.
-- foreign_link: a web link in the body leads to a site unrelated to the sender,
-- after a known redirect wrapper is unwrapped.
-- sender_was_stranger: when the mail arrived, the workspace had never written to
-- its sender.
--
-- NULL means not judged: anything but inbound mail, and mail captured before
-- these columns. The waiting queue reads NULL as no evidence.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN bcc_only boolean;
ALTER TABLE activity ADD COLUMN foreign_link boolean;
ALTER TABLE activity ADD COLUMN sender_was_stranger boolean;
