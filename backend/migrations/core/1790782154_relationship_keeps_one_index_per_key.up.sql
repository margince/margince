-- relationship carried three index layers over the same six foreign keys, so
-- every insert and update maintained thirty-one B-tree entries on the heaviest
-- write in the schema.
--
-- LAYER A, four traverse indexes, is subsumed by the history index on the same
-- column: an unpredicated index answers every query a predicated one on that
-- column answers, and the planner rechecks archived_at in the heap.
--
-- LAYER C's twelve single-column per-kind indexes sit on columns that already
-- carry both of the others. A per-kind index only wins when that kind is a
-- small slice of a large table, and nothing establishes that — there is no
-- production installation, so the distribution of `kind` is unknown and twelve
-- indexes were being maintained against a guess. One comes back the day
-- pg_stat_user_indexes shows a history index doing heavy work for a narrow
-- query, which is a minute's migration.
--
-- WHAT STAYS. The six history indexes: every foreign key here is ON DELETE
-- CASCADE and a partial index cannot answer a cascade, so dropping them would
-- put the sequential scans back. The seven unique partials: each enforces an
-- invariant no CHECK can express, which is correctness rather than speed. And
-- idx_rel_employer_contacts, which is two columns and answers a question none
-- of the others do.
SET LOCAL lock_timeout = '3s';

DROP INDEX idx_rel_traverse_company;
DROP INDEX idx_rel_traverse_contact;
DROP INDEX idx_rel_traverse_deal;
DROP INDEX idx_rel_traverse_project;

DROP INDEX idx_rel_company_contacts;
DROP INDEX idx_rel_company_projects;
DROP INDEX idx_rel_contact_companies;
DROP INDEX idx_rel_contact_projects;
DROP INDEX idx_rel_deal_stakeholders;
DROP INDEX idx_rel_partner_company;
DROP INDEX idx_rel_partner_counterparty;
DROP INDEX idx_rel_project_companies;
DROP INDEX idx_rel_project_stakeholders;
DROP INDEX idx_rel_stakeholder_deals;
DROP INDEX idx_rel_works_with_contact;
DROP INDEX idx_rel_works_with_counterparty;
