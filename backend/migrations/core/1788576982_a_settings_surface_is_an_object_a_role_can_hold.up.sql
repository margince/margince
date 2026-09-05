-- Thirteen settings surfaces become objects a role can hold.
--
-- Reach the installations that already exist. seedSystemRoles writes each role
-- document once at workspace creation and never re-syncs, so an object added to
-- the compiled defaults alone is granted to nobody who bootstrapped earlier — it
-- works on a fresh database and 403s everywhere else, permanently.
--
-- None of the thirteen has ever existed, so no role holds any of the keys and
-- the guard is absence. An operator who later hand-sets one keeps their setting.
--
-- Every role is named for every object, the zero grants written out rather than
-- left out. A role with no key at all is indistinguishable from a role the
-- backfill missed, and the next reader auditing this cannot tell a decision from
-- an omission.
SET LOCAL lock_timeout = '5s';

-- Inviting a member, reading the privileged roster, changing a role, issuing a
-- password link and deactivating an account. Admin alone: it is the authority
-- that reaches every other authority, so an installation delegates it by
-- deliberately editing a custom role, never by inheriting it here.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,user_admin}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'ops', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'user_admin';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,user_admin}',
        '{"create":true,"read":true,"update":true,"delete":true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'user_admin';

-- Ops READS the role directory — answering "why can this person not see that"
-- needs the policy in front of you — and changes nothing. A holder of the
-- editor can grant themselves anything the editor expresses, so writing it
-- stays with admin.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,role_admin}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'role_admin';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,role_admin}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'role_admin';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,role_admin}',
        '{"create":true,"read":true,"update":true,"delete":true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'role_admin';

-- Creating a team and moving members between teams. No delete on any role: a
-- team that existed is what a past record's visibility was resolved against.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,team_admin}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'ops', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'team_admin';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,team_admin}',
        '{"create":true,"read":true,"update":true,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'team_admin';

-- The privacy inbox. Read and update only — a subject request is raised by the
-- subject, never by an operator, and answering it is the update.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,privacy_request}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'ops', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'privacy_request';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,privacy_request}',
        '{"create":false,"read":true,"update":true,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'privacy_request';

-- Admin alone, and read is the only verb it will ever carry. Ops administers the
-- installation's wiring, not the record of who did what to whom.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,audit_log}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'ops', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'audit_log';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,audit_log}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'audit_log';

-- Queue depth, retry ladders and what capture's judgement queues hold. The
-- question an operator is on call for, so ops reads it.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,job_health}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'job_health';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,job_health}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'job_health';

-- Which extension units this installation composed. Read-only for everyone:
-- presence under extensions/ IS the enablement, so there is no runtime toggle
-- for a grant to gate.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,extension_access}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'extension_access';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,extension_access}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'extension_access';

-- Erasing the installation's data. Admin alone and never ops — the reset is the
-- one action no operator performs on somebody else's behalf. Read is whether
-- the reset is armed and what it would erase; delete is performing it.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,system_reset}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'ops', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'system_reset';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,system_reset}',
        '{"create":false,"read":false,"update":false,"delete":true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'system_reset';

-- Model calls, AI health and spend. Management reads it because the spend is
-- theirs to answer for. The routing that CHANGES it is ai_routing and is
-- untouched here.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,ai_diagnostics}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('manager', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'ai_diagnostics';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,ai_diagnostics}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'management', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'ai_diagnostics';

-- The consent-purpose vocabulary every capture and outreach decision is judged
-- against. Ops holds it in full because the purposes are wiring; management
-- reads what its team is bound by.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,consent_config}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('manager', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'consent_config';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,consent_config}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'consent_config';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,consent_config}',
        '{"create":true,"read":true,"update":true,"delete":true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'consent_config';

-- Which sign-in providers the installation offers. Management and ops read the
-- posture; changing who may enter the installation is admin.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,authentication_policy}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('manager', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'authentication_policy';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,authentication_policy}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'authentication_policy';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,authentication_policy}',
        '{"create":false,"read":true,"update":true,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'authentication_policy';

-- The workspace's OAuth applications — the client credentials a mailbox
-- connector is issued against. Split from capture_settings, which every sales
-- role holds, because a client secret is not a capture setting.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,oauth_application}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('manager', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'oauth_application';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,oauth_application}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'oauth_application';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,oauth_application}',
        '{"create":true,"read":true,"update":true,"delete":true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'oauth_application';

-- How many seats are used against how many are held. Non-commercial by
-- construction: the licensee and the entitlement stay on `license`, so
-- management plans headcount without reading the contract.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,seat_usage}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('manager', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'seat_usage';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,seat_usage}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'management', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'seat_usage';
