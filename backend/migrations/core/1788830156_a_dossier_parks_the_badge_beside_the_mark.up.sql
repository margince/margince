-- A cold-start dossier parks two marks, because the company it proposes is
-- drawn at two widths.
--
-- The installation's own company wears a wide lockup and a square badge
-- (organization.logo_icon_object_key), and the onboarding website read is the
-- one machine writer that can offer it either: it reads the site before the
-- record exists, so what it resolves waits on the dossier until a confirmation
-- binds it. The wide mark already had its pair of columns here; the badge gets
-- the same pair, in the same shape — the bucket path the bytes live at, and
-- the asset the read resolved them from — so the confirmation moves each slot
-- on the terms it already moves the first.
--
-- Adding a nullable column takes an ACCESS EXCLUSIVE lock on site_read for the
-- instant it rewrites the catalog entry. The bound is on ACQUIRING it: an open
-- transaction holding a conflicting lock would otherwise stall every dossier
-- write for as long as this is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE site_read
    ADD COLUMN logo_icon_object_key text,
    ADD COLUMN logo_icon_origin text;
