-- classification and relevance are columns of `company` that nothing writes.
--
-- classification was retired when the multi-valued split landed and
-- company_relationship_type replaced it: every row holds the default the
-- baseline gave it, the list filter that read it is already gone, and the
-- contract no longer offers the enum. What stood was the column, its CHECK and
-- an index over one value.
--
-- relevance never had a writer at all — no write path in the contract, no
-- screen, and the only readers were two SELECT lists carrying it to a
-- destination nobody read.
--
-- A column nothing writes is not inert. It is a column-by-column review's
-- subject, a value every reader has to rule out, and — for classification — a
-- default that reads as a judgement somebody made.
SET LOCAL lock_timeout = '3s';

DROP INDEX idx_company_class;

ALTER TABLE company
    DROP CONSTRAINT company_classification_check,
    DROP CONSTRAINT company_relevance_check,
    DROP COLUMN classification,
    DROP COLUMN relevance;
