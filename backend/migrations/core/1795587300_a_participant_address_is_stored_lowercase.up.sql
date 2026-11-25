-- The participant key reads two spellings of one address as two parties. The
-- check holds the lowercased, trimmed form; a data task corrects older rows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant ADD CONSTRAINT activity_participant_address_is_canonical
    CHECK (address = lower(btrim(address))) NOT VALID;

-- An unvalidated check still judges an older row on its next update, so this
-- lowercases the row first. A row that already holds that party folds into it.
-- A given answer beats none, then the later written row wins. The name follows
-- the answer, and the row keeps each roster seat and a member's relink.
CREATE FUNCTION store_participant_address_lowercase() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
 folded record;
BEGIN
 NEW.address := lower(btrim(NEW.address));
 WITH gone AS (
  DELETE FROM activity_participant twin
   WHERE twin.activity_id = NEW.activity_id AND twin.role = NEW.role AND twin.id <> NEW.id
     AND twin.user_id IS NOT DISTINCT FROM NEW.user_id
     AND twin.contact_id IS NOT DISTINCT FROM NEW.contact_id
     AND twin.channel_user_id IS NOT DISTINCT FROM NEW.channel_user_id
     AND twin.address = NEW.address
  RETURNING twin.id, twin.created_at AS written_at, twin.response, twin.display_name,
            twin.roster_seats, twin.roster_seat, twin.relinked_at, twin.past_party_cap
 ), spelling AS (
  SELECT * FROM gone
  UNION ALL
  SELECT NEW.id, CASE WHEN NEW.response IS DISTINCT FROM OLD.response THEN 'infinity' ELSE NEW.created_at END,
         NEW.response, NEW.display_name, NEW.roster_seats, NEW.roster_seat, NEW.relinked_at, NEW.past_party_cap
 ), ranked AS (
  SELECT s.*, row_number() OVER (ORDER BY COALESCE(s.response IN ('accepted', 'declined', 'tentative'), false) DESC,
                                          s.written_at DESC, s.id DESC) AS rank
    FROM spelling s
 )
 SELECT count(*) AS spellings,
        (array_agg(r.response ORDER BY r.rank) FILTER (WHERE r.response IS NOT NULL))[1] AS response,
        COALESCE((array_agg(r.display_name ORDER BY r.rank) FILTER (WHERE r.display_name <> ''))[1],
                 max(r.display_name)) AS display_name,
        ARRAY(SELECT DISTINCT listing.seat
                FROM ranked listed, unnest(array_remove(listed.roster_seats || listed.roster_seat, NULL)) AS listing(seat)
               ORDER BY 1) AS roster_seats,
        max(r.relinked_at) AS relinked_at,
        bool_and(r.past_party_cap) AS past_party_cap
   INTO folded
   FROM ranked r;
 IF folded.spellings > 1 THEN
  NEW.response := folded.response;
  NEW.display_name := folded.display_name;
  NEW.roster_seats := NULLIF(folded.roster_seats, '{}');
  NEW.roster_seat := NULL;
  NEW.relinked_at := folded.relinked_at;
  NEW.past_party_cap := folded.past_party_cap;
 END IF;
 RETURN NEW;
END;
$$;

CREATE TRIGGER address_is_stored_lowercase
 BEFORE UPDATE ON activity_participant
 FOR EACH ROW WHEN (NEW.address <> lower(btrim(NEW.address)))
 EXECUTE FUNCTION store_participant_address_lowercase();
