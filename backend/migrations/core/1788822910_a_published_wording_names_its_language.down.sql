-- Reverting drops the controller wordings the narrow index cannot hold.
--
-- By the time this runs, bootstrap has published each controller template in
-- every language this build speaks — three rows sharing (key, version). The old
-- index admits one, so recreating it without deleting the others fails with a
-- duplicate-key error and the whole rollback aborts.
--
-- The non-English rows go, not the English one. That leaves an installation
-- where it was before the migration: one wording per version, in the fallback
-- language, which is what every proof row written before this could have
-- pointed at anyway.
--
-- Scoped to the two CONTROLLER TEMPLATES rather than to every non-English
-- wording. A preference-centre sentence published per language by some later
-- writer is not this migration's to delete, and a rollback that swept one would
-- take evidence it never added.
--
-- WHEN THIS STOPS WORKING, and it should: consent_event.consent_text_version_id
-- references this table with the default ON DELETE RESTRICT, and nothing writes
-- that column today. The day a writer lands, a proof row naming a German
-- wording refuses this DELETE and the rollback aborts — correctly, because
-- deleting the row a proof points at is the falsification this table exists to
-- prevent. At that point the rollback is genuinely unsupported, and this file
-- should say so rather than be made to pass.

-- Bounded, so a transaction holding a conflicting lock stalls this migration
-- rather than every write to the table behind it.
SET LOCAL lock_timeout = '3s';

DELETE FROM consent_text_version
 WHERE key IN ('record_confirmation', 'consent_confirmation')
   AND locale IS NOT NULL
   AND locale <> 'en';

DROP INDEX IF EXISTS consent_text_version_published;

CREATE UNIQUE INDEX consent_text_version_published
    ON consent_text_version (key, version)
    WHERE published_at IS NOT NULL;
