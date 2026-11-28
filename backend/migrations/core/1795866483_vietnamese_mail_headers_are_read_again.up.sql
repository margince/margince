-- The write of the data task that reads stored mail again under the Vietnamese
-- header labels. It sets only the end, and the search trigger rebuilds the
-- value from it. No member edited the row, so its version and updated_at stay.
CREATE FUNCTION reread_activity_own_end(mails uuid[], own_ends integer[]) RETURNS integer
    LANGUAGE plpgsql
    AS $$
DECLARE
    moving text := current_setting('margince.last_activity_move', true);
    changed integer;
BEGIN
    PERFORM set_config('margince.last_activity_move', 'on', true);
    UPDATE activity a SET body_own_end = m.own_end
      FROM unnest(mails, own_ends) AS m(id, own_end)
     WHERE a.id = m.id AND a.restricted_at IS NULL AND a.body_own_end IS DISTINCT FROM m.own_end;
    GET DIAGNOSTICS changed = ROW_COUNT;
    PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
    RETURN changed;
EXCEPTION WHEN OTHERS THEN
    PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
    RAISE;
END;
$$;
