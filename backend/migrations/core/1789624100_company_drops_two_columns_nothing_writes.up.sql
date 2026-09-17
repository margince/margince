-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded, because ALTER TABLE takes ACCESS EXCLUSIVE on a table this
-- migration did not create.
SET LOCAL lock_timeout = '2s';

-- Two columns nothing writes, and the constraints and index that served them.
--
-- classification was retired when the multi-valued split landed and
-- company_relationship_type replaced it: one column could not answer both
-- "where does this account stand with us" and "what is it to us", because an
-- account is at one point in a sales motion while being several things at
-- once. The read paths went first; this is the column itself.
--
-- relevance was a 0-100 score with no writer, no write path in the contract
-- and no surface. The read path selected it into a variable it discarded.
--
-- Dropping rather than leaving them NULLable: a column that reads as a feature
-- and answers nothing costs every later reader the same investigation.
ALTER TABLE company
    DROP CONSTRAINT IF EXISTS company_classification_check,
    DROP CONSTRAINT IF EXISTS company_relevance_check,
    DROP COLUMN IF EXISTS classification,
    DROP COLUMN IF EXISTS relevance;

-- Goes with its column; no reader survives it.
DROP INDEX IF EXISTS idx_company_class;
