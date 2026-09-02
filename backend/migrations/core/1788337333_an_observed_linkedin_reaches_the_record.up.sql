-- An observed LinkedIn URL reaches the record it describes.
--
-- The enrichment passes (signature, card, site read, public search) store a
-- contact's LinkedIn URL as a person_profile_field evidence row, and until now
-- stopped there. person_social is what the rest of the product reads — the
-- person rail's LinkedIn row, the provider identifier resolver that decides
-- whether an enrichment vendor can look the contact up, the SAR export — so
-- every such contact showed a LinkedIn URL in its research section while the
-- record answered "none" everywhere else. The writer now fills the empty
-- social slot at capture time; this carries the rows captured before it did.
--
-- Additive only, exactly like the writer: a handle already on the record is
-- somebody's statement and is kept. The host predicate mirrors the writer's
-- isLinkedinURL gate (linkedin.com and its subdomains, with or without a
-- scheme) — a value that is not a profile link stays evidence-only.
INSERT INTO person_social (person_id, platform, handle)
SELECT f.person_id, 'linkedin', f.value
  FROM person_profile_field f
  JOIN person p ON p.id = f.person_id AND p.archived_at IS NULL
 WHERE f.field = 'linkedin'
   AND lower(f.value) ~ '^(https?://)?([a-z0-9-]+\.)?linkedin\.com(:[0-9]+)?(/|$)'
ON CONFLICT (person_id, platform) DO NOTHING;
