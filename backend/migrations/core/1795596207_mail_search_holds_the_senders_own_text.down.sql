-- The search value becomes a generated column over the whole body again. This
-- rewrites activity, because Postgres 16 cannot attach an expression in place.
SET LOCAL lock_timeout = '3s';

DROP FUNCTION settlement_evidence_activity_keys(text, text, text);

CREATE FUNCTION settlement_evidence_activity_keys(row_value activity) RETURNS bigint[]
    LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN row_value.kind IN ('email', 'message', 'call', 'meeting') THEN ARRAY[
        settlement_evidence_key('thread', row_value.thread_key),
        settlement_evidence_key('address', lower(btrim(row_value.counterparty_email)))] END
$$;

CREATE OR REPLACE FUNCTION settlement_evidence_share_activity() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM settlement_evidence_share(settlement_evidence_activity_keys(OLD));
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        PERFORM settlement_evidence_share(settlement_evidence_activity_keys(OLD) || settlement_evidence_activity_keys(NEW));
    ELSE
        PERFORM settlement_evidence_share(settlement_evidence_activity_keys(NEW));
    END IF;
    RETURN NEW;
END $$;

DROP FUNCTION rebuild_activity_search_text(uuid[], uuid[], integer[]);
DROP TRIGGER activity_search_tsv_update ON activity;
DROP TRIGGER activity_search_tsv_insert ON activity;
DROP FUNCTION activity_keeps_search_tsv();
DROP FUNCTION activity_search_tsv(text, text, integer, text);

ALTER TABLE activity DROP COLUMN search_tsv;
ALTER TABLE activity ADD COLUMN search_tsv tsvector GENERATED ALWAYS AS ((((setweight(to_tsvector('simple'::regconfig, f_unaccent(COALESCE(subject, ''::text))), 'A'::"char") || setweight(to_tsvector('simple'::regconfig, f_fold_apostrophes(COALESCE(subject, ''::text))), 'A'::"char")) || setweight(to_tsvector(activity_ts_config(language), f_unaccent(((COALESCE(subject, ''::text) || ' '::text) || COALESCE(body, ''::text)))), 'B'::"char")) || setweight(to_tsvector('simple'::regconfig, f_unaccent(COALESCE(body, ''::text))), 'C'::"char"))) STORED;
CREATE INDEX idx_activity_search ON activity USING gin (search_tsv);

ALTER TABLE activity DROP COLUMN body_own_end;
