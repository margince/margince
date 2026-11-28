SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION activity_keeps_search_tsv() RETURNS trigger
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

CREATE OR REPLACE TRIGGER activity_search_tsv_update
    BEFORE UPDATE OF subject, body, language, body_own_end, search_tsv ON activity
    FOR EACH ROW
    WHEN (ROW(OLD.subject, OLD.body, OLD.language, OLD.body_own_end, OLD.search_tsv)
          IS DISTINCT FROM ROW(NEW.subject, NEW.body, NEW.language, NEW.body_own_end, NEW.search_tsv))
    EXECUTE FUNCTION activity_keeps_search_tsv();
