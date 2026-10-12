-- `raw` is the unparsed upstream payload a row was built from, kept for replay
-- and debugging. Eight tables carried the column and one is given anything to
-- keep: activity, where a captured message's own payload lands.
--
-- On contact and lead the only statements naming it SET IT TO NULL — privacy
-- erasure scrubbing a column nothing ever fills, which is a promise about data
-- that cannot exist. On company, deal, partner, dedupe_candidate and project
-- there is no statement at all.
--
-- A column named by the schema-wide convention and filled by nothing is worse
-- than an absent one: a reader planning a replay finds the column, believes the
-- payload is there, and writes against rows that are all NULL.
--
-- activity keeps its column, the six statements that clear it — five under
-- privacy/ and capturenoise.go's — and the restriction trigger reading NEW.raw.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company DROP COLUMN raw;
ALTER TABLE contact DROP COLUMN raw;
ALTER TABLE deal DROP COLUMN raw;
ALTER TABLE dedupe_candidate DROP COLUMN raw;
ALTER TABLE lead DROP COLUMN raw;
ALTER TABLE partner DROP COLUMN raw;
ALTER TABLE project DROP COLUMN raw;
