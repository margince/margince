-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- An address or domain a key compares has one spelling: accents composed (NFC)
-- and no character that draws nothing. values.IdentifierText writes it; these
-- checks refuse a writer that skips it. The next migration validates them.
CREATE FUNCTION identifier_spelling(value text) RETURNS text
  LANGUAGE sql IMMUTABLE PARALLEL SAFE
  RETURN normalize(regexp_replace(value,
    '[\u0001-\u001F\u007F-\u009F\u00AD\u061C\u115F\u1160\u180E\u200B-\u200F\u202A-\u202E\u2060-\u2064\u2066-\u206F\u3164\uFEFF\uFFA0]',
    '', 'g'), NFC);

CREATE FUNCTION is_identifier_text(value text) RETURNS boolean
  LANGUAGE sql IMMUTABLE PARALLEL SAFE
  RETURN value = identifier_spelling(value);

ALTER TABLE app_user ADD CONSTRAINT app_user_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE capture_freemail_domain ADD CONSTRAINT capture_freemail_domain_domain_one_spelling CHECK (is_identifier_text(domain)) NOT VALID;
ALTER TABLE capture_pending_counterparty ADD CONSTRAINT capture_pending_counterparty_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE capture_sender_override ADD CONSTRAINT capture_sender_override_address_one_spelling CHECK (is_identifier_text(address)) NOT VALID;
ALTER TABLE communication_suppression ADD CONSTRAINT communication_suppression_address_one_spelling CHECK (is_identifier_text(address)) NOT VALID;
ALTER TABLE company_domain ADD CONSTRAINT company_domain_domain_one_spelling CHECK (is_identifier_text(domain)) NOT VALID;
ALTER TABLE company_domain_disposition ADD CONSTRAINT company_domain_disposition_domain_one_spelling CHECK (is_identifier_text(domain)) NOT VALID;
ALTER TABLE contact_email ADD CONSTRAINT contact_email_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE deal_room_participant ADD CONSTRAINT deal_room_participant_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE lead ADD CONSTRAINT lead_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE workspace_email_domain ADD CONSTRAINT workspace_email_domain_domain_one_spelling CHECK (is_identifier_text(domain)) NOT VALID;

-- A participant row is written in bulk by capture, so its older rows stay
-- unvalidated. The update trigger moves such a row to the one spelling first.
ALTER TABLE activity_participant ADD CONSTRAINT activity_participant_address_one_spelling
    CHECK (is_identifier_text(address)) NOT VALID;

CREATE OR REPLACE FUNCTION store_participant_address_lowercase() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
 folded record;
BEGIN
 NEW.address := lower(btrim(identifier_spelling(NEW.address)));
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

DROP TRIGGER address_is_stored_lowercase ON activity_participant;
CREATE TRIGGER address_is_stored_lowercase
 BEFORE UPDATE ON activity_participant
 FOR EACH ROW WHEN (NEW.address <> lower(btrim(identifier_spelling(NEW.address))))
 EXECUTE FUNCTION store_participant_address_lowercase();
