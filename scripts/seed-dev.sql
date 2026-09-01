-- seed-dev.sql — the dev-database seed for demo data that has no public API.
--
-- Companion to scripts/seed-dev.sh (the API seed for people/orgs/deals). This
-- file holds dev/demo data that can only be written directly to the database —
-- reference tables and config the product intentionally exposes no REST/MCP
-- endpoint for. It is part of the default dev-env init: `make dev` applies it on
-- boot and `make seed-dev` re-applies it, both AFTER the API seed has created
-- the demo workspace. So a developer runs `make dev && make seed-dev` and every
-- surface is testable with the necessary data pre-filled. Idempotent — safe to
-- re-run; extend it as more API-less demo data or settings are needed.
--
-- Seeds two things, demo-workspace only:
--   1. FX rates (fx_rate is an exchange-rate feed — no API, no audit_log/
--      event_outbox, and the product never invents a rate at runtime, so a
--      non-EUR deal cannot be won without a seeded rate). Never seeded at
--      workspace bootstrap, so real workspaces keep the honest "no rate → 422,
--      never rate=1" behaviour.
--   2. The RBAC demo fixture the sharing/roles surfaces need: two non-admin
--      seats (Rep One, team-scoped; Rep Two, own-scoped), the DACH Sales team,
--      their role assignments, and admin-ownership of the API-seeded records so
--      row scope actually restricts them. See the demo-accounts manifest below.
--
-- Requires the compose stack's Postgres (make seed-dev-db runs it as
-- margince_owner; make dev applies it over the dev owner DSN).
--
-- Demo accounts — the ONE place the dev login credentials are described. The
-- `make dev` ready banner (scripts/dev.sh) prints the lines between the markers
-- below verbatim, so there is no second copy to drift. Fixed demo values on a
-- throwaway localhost DB (admin is bootstrapped by scripts/seed-dev.sh; the two
-- reps by this file) — never real credentials. Keep this in sync with the
-- INSERTs below.
-- DEMO-ACCOUNTS-BEGIN
-- workspace demo-workspace  ·  password (all three): demo-password-123
-- admin@demo.test   admin       — sees every record
-- rep@demo.test     rep         — team-scoped (team DACH Sales)
-- rep2@demo.test    individual  — own-scoped, no team (sees only what's shared)
-- DEMO-ACCOUNTS-END

BEGIN;

-- installation_workspace resolves THE installation's workspace, and refuses any
-- other answer. One spelling for all three blocks below, because the question is
-- one question and three copies of it are three chances to drift.
--
-- It mirrors identity.activeWorkspaces, the production authority: archived rows
-- are excluded, and MORE THAN ONE live row raises rather than being picked
-- between. `LIMIT 1` would have made this script seed whichever workspace
-- happened to be oldest — silently, and with no tenant column left on core for
-- the mismatch to surface on afterwards.
--
-- pg_temp, so it lives for this session only and cannot leak into the schema
-- these scripts are pointed at.
CREATE FUNCTION pg_temp.installation_workspace() RETURNS uuid AS $fn$
DECLARE ws uuid;
BEGIN
  SELECT id INTO STRICT ws FROM workspace WHERE archived_at IS NULL;
  RETURN ws;
EXCEPTION
  -- Absent is a state the caller handles (nothing bootstrapped yet); ambiguous
  -- is not one this script may guess its way through.
  WHEN no_data_found THEN
    RETURN NULL;
  WHEN too_many_rows THEN
    RAISE EXCEPTION 'seed-dev.sql: more than one live workspace, so there is no such thing as THE installation''s workspace here — refusing rather than seeding whichever is oldest';
END;
$fn$ LANGUAGE plpgsql;

DO $$
DECLARE
  ws uuid;
BEGIN
  ws := pg_temp.installation_workspace();
  IF ws IS NULL THEN
    RAISE NOTICE 'seed-dev.sql: no live workspace — run make seed-dev first';
    RETURN;
  END IF;

  -- FX rates: base currency is EUR; seed the three other UI currencies
  -- (USD/GBP/CHF) dated today so a close on or after today finds a rate.
  -- Representative demo values — not a live quote.
  INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
  VALUES
    ('USD', 'EUR', 0.92, CURRENT_DATE),
    ('GBP', 'EUR', 1.17, CURRENT_DATE),
    ('CHF', 'EUR', 1.04, CURRENT_DATE)
  ON CONFLICT (from_currency, to_currency, rate_date)
    DO UPDATE SET rate = EXCLUDED.rate;

  RAISE NOTICE 'seed-dev.sql: FX rates USD/GBP/CHF → EUR seeded for demo-workspace (rate_date=%)', CURRENT_DATE;
END $$;

DO $$
DECLARE
  ws uuid;
  admin_id uuid;
  admin_hash text;
  rep_id uuid;
  rep2_id uuid;
  dach_team_id uuid;
BEGIN
  ws := pg_temp.installation_workspace();
  IF ws IS NULL THEN
    RAISE NOTICE 'seed-dev.sql: no live workspace — run make seed-dev first';
    RETURN;
  END IF;

  -- The demo admin is bootstrapped through the public API (scripts/seed-dev.sh),
  -- never here — reuse its password_hash verbatim so the 2nd seat shares the
  -- demo password, without re-implementing Argon2id hashing in SQL.
  SELECT id, password_hash INTO admin_id, admin_hash
    FROM app_user
    WHERE lower(email) = lower('admin@demo.test');
  IF admin_id IS NULL THEN
    RAISE NOTICE 'seed-dev.sql: no admin@demo.test user — run make seed-dev first';
    RETURN;
  END IF;

  -- The API seed (seed-dev.sh) creates people/orgs/deals with NO owner, and an
  -- ownerless row is shared — visible at EVERY row scope. That would let the
  -- own-scoped Rep Two (below) see everything and make record sharing
  -- unobservable. Make Demo Admin the owner of every ownerless seeded record so
  -- row scope actually bites (captured_by is already the admin). Idempotent —
  -- only touches rows that are still ownerless.
  --
  -- These reach every ownerless row in the INSTALLATION, not a tenant's subset:
  -- ADR-0091 §8 phase D is taking the tenant column off these tables, and where
  -- it is already gone there is no narrower set to name. What bounds the blast
  -- radius is the guard at the top of this block, not a predicate here — the
  -- whole DO block returns unless a live workspace and an admin@demo.test
  -- user both exist, which is the demo installation and not
  -- anything else. A seed that creates users with a published password was
  -- never safe to point at real data; the tenant predicate narrowed the damage
  -- but was never what made it safe.
  UPDATE person       SET owner_id = admin_id WHERE owner_id IS NULL;
  UPDATE organization SET owner_id = admin_id WHERE owner_id IS NULL;
  UPDATE deal         SET owner_id = admin_id WHERE owner_id IS NULL;
  UPDATE lead         SET owner_id = admin_id WHERE owner_id IS NULL;

  -- 2nd full-seat user so the Share picker / "who has access" have a real
  -- subject beyond the lone admin.
  INSERT INTO app_user (email, password_hash, display_name, seat_type, status)
  VALUES ('rep@demo.test', admin_hash, 'Rep One', 'full', 'active')
  ON CONFLICT (lower(email)) DO NOTHING;

  SELECT id INTO rep_id
    FROM app_user
    WHERE lower(email) = lower('rep@demo.test');

  -- A team with admin + Rep One as members, so the roster picker and the
  -- "who has access" list have a demonstrable, non-trivial membership.
  INSERT INTO team (name)
  VALUES ('DACH Sales')
  ON CONFLICT (name) DO NOTHING;

  SELECT id INTO dach_team_id
    FROM team
    WHERE name = 'DACH Sales';

  INSERT INTO team_membership (team_id, user_id)
  VALUES
    (dach_team_id, admin_id),
    (dach_team_id, rep_id)
  ON CONFLICT (team_id, user_id) DO NOTHING;

  -- A seat with no role_assignment has NO permissions — every object check
  -- (pipeline.read, deal.read, …) fails closed, so Rep One can't even load a
  -- list, let alone see a record shared with them. Assign the 'rep' system role
  -- (seeded at workspace bootstrap): team-scoped read/write, so Rep One sees the
  -- team's records plus whatever is explicitly shared. Idempotent via NOT EXISTS
  -- (role_assignment's uniqueness is an expression index over COALESCE(team_id)).
  INSERT INTO role_assignment (role_id, user_id)
  SELECT r.id, rep_id
    FROM role r
    WHERE r.key = 'rep'
      AND NOT EXISTS (
        SELECT 1 FROM role_assignment ra
        WHERE ra.user_id = rep_id AND ra.role_id = r.id AND ra.team_id IS NULL
      );

  -- An own-scoped counterpart to the team-scoped 'rep': identical object reach,
  -- narrower row scope, so a holder sees ONLY their own records plus whatever is
  -- explicitly shared with them. Cloned from 'rep' (object grants stay in
  -- lockstep) with row_scope overridden to 'own'. Not a system role — it exists
  -- so the individual demo seat (Rep Two, below) makes record sharing OBSERVABLE:
  -- with no team and no owned records, a grant is the sole reason a record shows.
  INSERT INTO role (key, name, is_system, permissions)
  SELECT 'individual', 'Individual (own records)', false,
         jsonb_set(r.permissions, '{row_scope}', '"own"'::jsonb)
    FROM role r
    WHERE r.key = 'rep'
  ON CONFLICT (key) DO NOTHING;

  -- Rep Two: an individual contributor — own-scoped, in NO team. Both reps read
  -- the whole workspace and own nothing in it; the difference is the GRANT below,
  -- which is handed to Rep One and not to Rep Two.
  INSERT INTO app_user (email, password_hash, display_name, seat_type, status)
  VALUES ('rep2@demo.test', admin_hash, 'Rep Two', 'full', 'active')
  ON CONFLICT (lower(email)) DO NOTHING;

  SELECT id INTO rep2_id
    FROM app_user
    WHERE lower(email) = lower('rep2@demo.test');

  INSERT INTO role_assignment (role_id, user_id)
  SELECT r.id, rep2_id
    FROM role r
    WHERE r.key = 'individual'
      AND NOT EXISTS (
        SELECT 1 FROM role_assignment ra
        WHERE ra.user_id = rep2_id AND ra.role_id = r.id AND ra.team_id IS NULL
      );

  -- ONE of Demo Admin's people, shared with Rep One at `write`.
  --
  -- This is what makes sharing observable now that team membership does not
  -- grant it. Rep One and Rep Two are both own-scoped and own nothing, so they
  -- read the same workspace and edit the same nothing — except this one record,
  -- whose only reason for being editable is the grant. Take the grant away and
  -- the two seats are identical.
  --
  -- granted_by is the admin: a grant is passed on by somebody who could change
  -- the row themselves, which is the rule CreateRecordGrant enforces.
  INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by, reason)
  SELECT 'person', p.id, 'user', rep_id, 'write', admin_id,
         'seed-dev: the one record that makes a write share observable'
    FROM person p
   WHERE p.owner_id = admin_id AND p.archived_at IS NULL
   ORDER BY p.created_at, p.id
   LIMIT 1
  ON CONFLICT DO NOTHING;

  RAISE NOTICE 'seed-dev.sql: rep@demo.test (rep, own-scope, one write grant) + rep2@demo.test (individual, own-scope, no grant) seeded for demo-workspace';
END $$;

-- The finance mirror's demo source (ADR-0083/A128).
--
-- One offline_demo connection and a link per demo customer, so the finance
-- card has something to fill in: without a link the card sits in `unmapped`
-- forever, which is honest but demonstrates nothing.
--
-- The invoices are NOT seeded here. They come from the sync pass, which is
-- what makes this a demonstration of the real path rather than a table of
-- rows nobody produced — and it is the only way the hash discipline, the
-- credit-note placement and the derived statuses are exercised at all.
DO $$
DECLARE
  ws   uuid;
  conn uuid;
  org  RECORD;
BEGIN
  ws := pg_temp.installation_workspace();
  IF ws IS NULL THEN
    RETURN;
  END IF;
  SELECT id INTO conn
    FROM finance_connection
   WHERE provider = 'offline_demo' AND archived_at IS NULL;

  IF conn IS NULL THEN
    INSERT INTO finance_connection
           (provider, status, credential_ref, source, captured_by)
    VALUES ('offline_demo', 'active', 'offline://demo', 'system', 'system:seed')
    RETURNING id INTO conn;
  END IF;

  -- Customers only. A target or a prospect has never been invoiced, and the
  -- card is absent for them by design (FIN-AC-3) — linking one would put a
  -- ledger behind a company we have never billed.
  FOR org IN
    SELECT id, display_name FROM organization
     WHERE archived_at IS NULL
       AND lifecycle = 'customer'
  LOOP
    INSERT INTO finance_customer_link
           (connection_id, organization_id, external_customer_id,
            sync_hash, source, captured_by)
    VALUES (conn, org.id, 'DEMO-' || left(replace(org.id::text, '-', ''), 8),
            'seed', 'system', 'system:seed')
    ON CONFLICT DO NOTHING;
  END LOOP;

  RAISE NOTICE 'seed-dev.sql: offline_demo finance connection + customer links seeded; the sweep fills the ledger';
END $$;

-- 4. The one piece of Worklist work no endpoint can write: a customer waiting.
--
-- A waiting row is the newest inbound of a thread with no later outbound, and
-- "thread" means thread_key — which the create endpoint refuses on purpose. A
-- client naming its own thread could silence an unrelated conversation, so
-- capture stamps it and nothing else may. That leaves this fixture with no API
-- to go through, which is what this file is for.
--
-- Everything else the Worklist shows is seeded through the real endpoints in
-- seed-dev.sh, where it belongs.
--
-- Dated relative to now, so a seed run next month still demonstrates a two-day
-- wait rather than a stale one past the freshness horizon.
DO $$
DECLARE
  person_row uuid;
  activity_row uuid;
  capturer text;
BEGIN
  SELECT id INTO person_row FROM person
   WHERE full_name = 'Alice Müller' AND archived_at IS NULL
   ORDER BY created_at LIMIT 1;
  IF person_row IS NULL THEN
    RAISE NOTICE 'seed-dev.sql: no Alice Müller yet — run the API seed first; skipping the waiting customer';
    RETURN;
  END IF;
  -- Idempotent on the thread key, like every other block here.
  IF EXISTS (SELECT 1 FROM activity WHERE thread_key = 'seed-retrofit-pricing') THEN
    RAISE NOTICE 'seed-dev.sql: the waiting customer is already seeded';
    RETURN;
  END IF;
  SELECT 'human:' || id INTO capturer FROM app_user
   WHERE email = 'admin@demo.test' LIMIT 1;

  activity_row := uuidv7();
  INSERT INTO activity (id, kind, direction, subject, body, occurred_at, is_done,
                        source, captured_by, version, created_at, updated_at,
                        counterparty_outbound_attested, thread_key, audience)
  VALUES (activity_row, 'email', 'inbound',
          'Re: pricing for the retrofit',
          'Could you confirm the implementation cost before Friday?',
          now() - interval '2 days', false, 'system',
          coalesce(capturer, 'system:seed'), 1, now(), now(), false,
          'seed-retrofit-pricing', 'workspace');
  -- Filed under a person, which is what makes it SALES mail rather than a rep's
  -- own correspondence: the lane requires a link to a record the workspace
  -- sells to.
  INSERT INTO activity_link (activity_id, entity_type, person_id)
  VALUES (activity_row, 'person', person_row);
  -- Who wrote, so the lane can tell a person from a notification service.
  INSERT INTO activity_participant (activity_id, role, address, person_id)
  VALUES (activity_row, 'from', 'alice@demo.test', person_row);

  RAISE NOTICE 'seed-dev.sql: a customer is waiting on the Worklist';
END $$;

COMMIT;
