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
--
-- The subjects are LOCKED, in id order, before the insert. Art. 17 erasure
-- and retention anonymization hold the person row while they clear its
-- children, so an unlocked snapshot here could commit a fresh person_social
-- row for a subject erased in the same instant. Id order is the order
-- storekit's pair lock takes person rows in, so a concurrent merge and this
-- pass cannot deadlock. The wait is bounded: an eraser holds its rows for
-- moments, and failing loudly beats promoting into an erasure.
SET LOCAL lock_timeout = '3s';

WITH subject AS (
    SELECT p.id, f.value
      FROM person_profile_field f
      JOIN person p ON p.id = f.person_id AND p.archived_at IS NULL
     WHERE f.field = 'linkedin'
       AND lower(f.value) ~ '^(https?://)?([a-z0-9-]+\.)?linkedin\.com(:[0-9]+)?(/|$)'
     ORDER BY p.id
       FOR NO KEY UPDATE OF p
),
promoted AS (
    INSERT INTO person_social (person_id, platform, handle)
    SELECT id, 'linkedin', value FROM subject
    ON CONFLICT (person_id, platform) DO NOTHING
    RETURNING person_id
)
-- The bump is what the runtime writer does after the same insert: the social
-- set is part of the person aggregate, and a browser holding a pre-promotion
-- version token could otherwise replace the whole set it never saw, silently
-- dropping the handle just promoted. The row trigger advances the version.
UPDATE person SET updated_at = now()
 WHERE id IN (SELECT person_id FROM promoted);
