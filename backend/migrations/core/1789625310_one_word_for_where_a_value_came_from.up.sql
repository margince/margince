-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- One word for "where did this value come from".
--
-- Across the schema it is `source` 60 times, `provider` 14 and `origin` 4.
-- Three words for one question means a reader checks which spelling this table
-- chose before they can read the row. company.source already exists and names
-- the record's own provenance, so these three do not collide with it: they say
-- where the LOGO and the COORDINATES came from, one level down.
--
-- captured_by is untouched. It holds a principal string (human:… , agent:…),
-- which is who, not where from.
ALTER TABLE company RENAME COLUMN logo_origin TO logo_source;
ALTER TABLE company RENAME COLUMN logo_icon_origin TO logo_icon_source;
ALTER TABLE company RENAME COLUMN geocode_provider TO geocode_source;

-- A logo is a file AND where it came from, or it is neither.
--
-- Nothing stopped a row holding an object key with no source, which reads as a
-- logo nobody can attribute, or a source with no file, which reads as an
-- attribution for nothing. Both are states the writers never intend and no
-- reader handles.
ALTER TABLE company
    ADD CONSTRAINT company_logo_pair_is_whole
        CHECK ((logo_object_key IS NULL) = (logo_source IS NULL)),
    ADD CONSTRAINT company_logo_icon_pair_is_whole
        CHECK ((logo_icon_object_key IS NULL) = (logo_icon_source IS NULL));
