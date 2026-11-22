-- Mail search reads the text its sender wrote, up to where Go found the quoted
-- earlier mail. DROP EXPRESSION keeps the stored values and rewrites no row.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN body_own_end integer;

COMMENT ON COLUMN activity.body_own_end IS
    'How many characters of a mail body, from its start, the sender wrote. NULL reads the whole body.';

ALTER TABLE activity ALTER COLUMN search_tsv DROP EXPRESSION;

-- The body is read once when the language has no dictionary of its own: a
-- second simple pass would repeat the words the first one holds.
CREATE FUNCTION activity_search_tsv(subject text, body text, body_own_end integer, language text) RETURNS tsvector
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    RETURN setweight(to_tsvector('simple'::regconfig, f_unaccent(COALESCE(subject, ''))), 'A')
        || setweight(to_tsvector('simple'::regconfig, f_fold_apostrophes(COALESCE(subject, ''))), 'A')
        || setweight(to_tsvector(activity_ts_config(language),
               f_unaccent(COALESCE(subject, '') || ' ' || COALESCE(left(body, body_own_end), body, ''))), 'B')
        || CASE WHEN activity_ts_config(language) = 'simple'::regconfig THEN ''::tsvector
                ELSE setweight(to_tsvector('simple'::regconfig,
                         f_unaccent(COALESCE(left(body, body_own_end), body, ''))), 'C') END;

-- An equal value is left unassigned, so the row keeps its stored copy.
CREATE FUNCTION activity_keeps_search_tsv() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    fresh tsvector := activity_search_tsv(NEW.subject, NEW.body, NEW.body_own_end, NEW.language);
BEGIN
    IF NEW.search_tsv IS DISTINCT FROM fresh THEN
        NEW.search_tsv := fresh;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER activity_search_tsv_insert BEFORE INSERT ON activity
    FOR EACH ROW EXECUTE FUNCTION activity_keeps_search_tsv();

-- A statement that names none of these columns never fires this, and one that
-- names them unchanged stops at the WHEN.
CREATE TRIGGER activity_search_tsv_update
    BEFORE UPDATE OF subject, body, language, body_own_end, search_tsv ON activity
    FOR EACH ROW
    WHEN (ROW(OLD.subject, OLD.body, OLD.language, OLD.body_own_end, OLD.search_tsv)
          IS DISTINCT FROM ROW(NEW.subject, NEW.body, NEW.language, NEW.body_own_end, NEW.search_tsv))
    EXECUTE FUNCTION activity_keeps_search_tsv();

-- The write of the data task that fills stored rows. No member edited the row,
-- so its version and updated_at stay. A held row refuses every update.
CREATE FUNCTION rebuild_activity_search_text(inspected uuid[], mails uuid[], own_ends integer[]) RETURNS integer
    LANGUAGE plpgsql
    AS $$
DECLARE
    moving text := current_setting('margince.last_activity_move', true);
    filled integer;
    rebuilt integer;
BEGIN
    PERFORM set_config('margince.last_activity_move', 'on', true);
    UPDATE activity a SET body_own_end = m.own_end
      FROM unnest(mails, own_ends) AS m(id, own_end)
     WHERE a.id = m.id AND a.body_own_end IS NULL AND a.restricted_at IS NULL;
    GET DIAGNOSTICS filled = ROW_COUNT;
    UPDATE activity a SET search_tsv = activity_search_tsv(a.subject, a.body, a.body_own_end, a.language)
     WHERE a.id = ANY(inspected) AND a.restricted_at IS NULL
       AND a.search_tsv IS DISTINCT FROM activity_search_tsv(a.subject, a.body, a.body_own_end, a.language);
    GET DIAGNOSTICS rebuilt = ROW_COUNT;
    PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
    RETURN filled + rebuilt;
EXCEPTION WHEN OTHERS THEN
    PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
    RAISE;
END;
$$;

-- A whole row handed to a function is copied with its out-of-line values read
-- in, and the trigger then stores the body and the search value again. The
-- keys read three columns, so the row is left as it came.
DROP FUNCTION settlement_evidence_activity_keys(activity);

CREATE FUNCTION settlement_evidence_activity_keys(kind text, thread_key text, counterparty_email text) RETURNS bigint[]
    LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN kind IN ('email', 'message', 'call', 'meeting') THEN ARRAY[
        settlement_evidence_key('thread', thread_key),
        settlement_evidence_key('address', lower(btrim(counterparty_email)))] END
$$;

CREATE OR REPLACE FUNCTION settlement_evidence_share_activity() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM settlement_evidence_share(settlement_evidence_activity_keys(OLD.kind, OLD.thread_key, OLD.counterparty_email));
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        PERFORM settlement_evidence_share(
            settlement_evidence_activity_keys(OLD.kind, OLD.thread_key, OLD.counterparty_email)
            || settlement_evidence_activity_keys(NEW.kind, NEW.thread_key, NEW.counterparty_email));
    ELSE
        PERFORM settlement_evidence_share(settlement_evidence_activity_keys(NEW.kind, NEW.thread_key, NEW.counterparty_email));
    END IF;
    RETURN NEW;
END $$;
