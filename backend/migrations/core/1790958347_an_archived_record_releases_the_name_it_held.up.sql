-- A tombstoned row is invisible in the product and was still holding its name.
--
-- Four unconditional unique indexes on tables that carry archived_at. The archived
-- row cannot be seen, opened or renamed, so the conflict named a record the user has
-- no way to reach — and the only way out was to invent a different name.
--
-- The house style is already here: 56 of 104 unique indexes on tombstoned tables
-- carry the predicate, and offer_template holds uq_offer_template_default
-- (… WHERE is_default AND archived_at IS NULL) in the same table as the defect.
--
-- NOT every unconditional unique on a tombstoned table is a defect, and the four
-- left alone are left alone for reasons of their own:
--
--   role.role_key_unique stays. field_mask.role_key is a bare text column with no
--   foreign key — it names a role BY VALUE — so two roles sharing a key, even with
--   one archived, makes every mask filed under that string ambiguous.
--
--   custom_field is not here at all, for a reason the schema does not show. Its
--   create path derives the physical column as cf_<slug> and takes no override, and
--   refuseTakenColumn refuses a column an archived field still owns — correctly,
--   since that column holds the archived data. So releasing the slug needs the
--   column to be allowed to differ from it, which is a change to another module's
--   DDL path rather than to an index. #6598 carries it.
--
--   consent_purpose, app_user and oauth_client each carry an upsert that infers on
--   the key this would narrow, and the predicate changes what that upsert MEANS —
--   it would file a second row beside the archived one rather than reviving it.
--   Which of those two a seeder wants is a question about the product; #6598
--   carries it.
SET LOCAL lock_timeout = '3s';

-- Dropped as constraints, recreated as indexes: a UNIQUE constraint takes no
-- predicate, so the partial shape only exists as an index.
ALTER TABLE pipeline DROP CONSTRAINT pipeline_name_unique;
CREATE UNIQUE INDEX pipeline_name_unique ON pipeline (name) WHERE archived_at IS NULL;

ALTER TABLE team DROP CONSTRAINT team_name_unique;
CREATE UNIQUE INDEX team_name_unique ON team (name) WHERE archived_at IS NULL;

ALTER TABLE offer_template DROP CONSTRAINT offer_template_name_unique;
CREATE UNIQUE INDEX offer_template_name_unique ON offer_template (name) WHERE archived_at IS NULL;
