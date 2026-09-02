-- Rebuilds the retired quota table, empty, exactly as 0001_baseline created it,
-- and gives the seeded roles their grant back.
--
-- LOSSY IN TWO WAYS, both irreducible. The targets themselves are gone: the up
-- half dropped them with the table, and an empty table is what an installation
-- that never set one has. And a role that carried a NON-DEFAULT quota grant --
-- an operator-defined role, or a seeded one an admin had edited -- gets the
-- seeded default back rather than what it actually held, because the up half
-- recorded nothing about the grants it stripped. Restoring a shape nobody saved
-- is not something a down migration can do honestly.
SET LOCAL lock_timeout = '5s';

CREATE TABLE quota (
    id uuid DEFAULT uuidv7() NOT NULL,
    owner_id uuid,
    team_id uuid,
    period_start date NOT NULL,
    period_end date NOT NULL,
    target_minor bigint NOT NULL,
    currency char(3) NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    archived_at timestamptz,
    CONSTRAINT quota_pkey PRIMARY KEY (id),
    CONSTRAINT quota_currency_check CHECK ((currency ~ '^[A-Z]{3}$'::text)),
    CONSTRAINT quota_owner_xor_team CHECK (((owner_id IS NOT NULL) <> (team_id IS NOT NULL))),
    CONSTRAINT quota_period_valid CHECK ((period_end >= period_start)),
    CONSTRAINT quota_target_nonneg CHECK ((target_minor >= 0)),
    CONSTRAINT quota_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id) ON DELETE SET NULL,
    CONSTRAINT quota_team_id_fkey FOREIGN KEY (team_id) REFERENCES team(id) ON DELETE SET NULL
);

CREATE INDEX idx_quota_owner ON quota USING btree (owner_id) WHERE (owner_id IS NOT NULL);
CREATE INDEX idx_quota_team ON quota USING btree (team_id) WHERE (team_id IS NOT NULL);

CREATE TRIGGER trg_quota_updated BEFORE UPDATE ON quota
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE quota TO margince_app;

UPDATE role
   SET permissions = jsonb_set(permissions, '{objects,quota}',
       '{"create": true, "read": true, "update": true, "delete": true}'::jsonb, true)
 WHERE is_system AND key IN ('admin', 'ops')
   AND permissions ? 'objects' AND NOT (permissions -> 'objects') ? 'quota';

UPDATE role
   SET permissions = jsonb_set(permissions, '{objects,quota}',
       '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
 WHERE is_system AND key IN ('management', 'manager', 'read_only', 'rep')
   AND permissions ? 'objects' AND NOT (permissions -> 'objects') ? 'quota';
