-- The seat that sent an invitation this mailbox received, when its From is that
-- seat's login at an own domain and the receiving server passed DMARC for it.
--
-- It names an author and nothing else. No content or discovery gate reads it,
-- so naming a colleague never lets them read a copy they could not read before.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity ADD COLUMN invitation_sender uuid
  REFERENCES app_user(id) ON DELETE SET NULL;
