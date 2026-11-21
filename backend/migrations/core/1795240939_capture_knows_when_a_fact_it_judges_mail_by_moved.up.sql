-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- One counter that moves whenever a fact capture judges mail by moves: the
-- consumer-mail list, the own domains, the members' addresses, a seat's
-- identities or a mailbox's sharing setting. An import run keeps what it read
-- while the counter stands, so each message no longer reads it all again.
--
-- A sequence, never a clock: two processes write and read it.
SET LOCAL lock_timeout = '3s';

CREATE SEQUENCE IF NOT EXISTS capture_facts_version_seq;

-- The default privileges cover tables only, so the app role needs USAGE to call
-- nextval(). Conditional because a throwaway test database has no app role.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
    GRANT USAGE, SELECT ON SEQUENCE capture_facts_version_seq TO margince_app;
  END IF;
END $$;
