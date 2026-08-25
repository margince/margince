-- Every read of the audit spine stops at the newest scrub tombstone: audit_log
-- is append-only, so an Art. 17 erase cannot rewrite the images it certifies
-- gone and the reads enforce the boundary instead.
--
-- Proving a scrub's ABSENCE is what needs this. With only idx_audit_entity and
-- idx_audit_time to work from, the planner walks every audit row newer than the
-- one being judged and filters on the verb — measured at 19,015 rows removed per
-- judged row, and 22,671 buffer reads for one deep page against 182 for the
-- first. That cost is not bounded by the page limit and grows with the table,
-- which is the shape that stops being noticeable in review and starts being
-- noticeable in production.
--
-- Partial, on the three scrub verbs, because the absence being proved is of
-- those verbs alone — a full index would carry every ordinary update to answer a
-- question none of them can. That also makes the build cheap: it indexes the
-- scrub rows, which are a small fraction of the table.
--
-- Not CONCURRENTLY: a migration runs in one transaction and CONCURRENTLY forbids
-- that (see 1787320004's note on the same point), so this holds a write-blocking
-- build for its duration — bounded by the partial predicate rather than by the
-- whole of audit_log.
CREATE INDEX IF NOT EXISTS idx_audit_scrub_boundary
    ON audit_log (entity_type, entity_id, occurred_at DESC, id DESC)
    WHERE action IN ('erase', 'anonymize', 'restrict');
