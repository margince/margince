-- An activity's duration is unknown (NULL) or zero and more. NOT VALID: rows
-- written before the API refused a negative duration may hold one, and the
-- negative counts data task clears them.
SET LOCAL lock_timeout = '5s';
ALTER TABLE activity ADD CONSTRAINT activity_duration_not_negative
    CHECK (duration_seconds IS NULL OR duration_seconds >= 0) NOT VALID;

-- Postgres checks a NOT VALID floor on every update, so an old negative row
-- could no longer be edited. An update that carries such a duration over
-- unchanged clears it instead, before the check runs.
CREATE FUNCTION activity_clears_negative_duration() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    NEW.duration_seconds := NULL;
    RETURN NEW;
END;
$$;

CREATE TRIGGER activity_clears_negative_duration
    BEFORE UPDATE ON activity
    FOR EACH ROW
    WHEN (OLD.duration_seconds < 0 AND NEW.duration_seconds IS NOT DISTINCT FROM OLD.duration_seconds)
    EXECUTE FUNCTION activity_clears_negative_duration();
