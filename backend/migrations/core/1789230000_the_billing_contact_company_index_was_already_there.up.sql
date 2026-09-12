-- The billing-contact indexes were already there, under other names.
--
-- 1789222000 added three indexes for one kind. Two of them earn nothing, which
-- a look at the table's existing 32 would have shown before they were written.
--
-- rel_billing_contact_by_company is (company_id, role) with a partial
-- predicate, and uq_rel_billing_contact is (company_id, contact_id, role) with
-- the same predicate — so the unique index already serves every lookup that
-- starts from the company.
--
-- rel_billing_contact_by_contact is (contact_id), and idx_rel_history_contact
-- has been plain (contact_id) over EVERY kind since the table was built. A
-- partial copy of an index that already covers the same column unfiltered adds
-- nothing a billing read can use.
--
-- Both verified on a seeded table rather than an empty one, where the planner
-- will pick almost anything: with each dropped in turn, the read it was meant
-- to serve still plans as an index scan, over uq_rel_billing_contact and
-- idx_rel_history_contact respectively.
--
-- Worth removing rather than leaving as harmless spare weight. An empty
-- `relationship` reached 270336 bytes across 32 indexes, and the test harness
-- gives an unbaselined table 262144 bytes of slack before it stops DELETEing
-- and starts TRUNCATEing it on every reset. These two are what crossed it, so
-- their cost is paid by every integration test in the suite, forever, to answer
-- queries two older indexes already answer.

SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS rel_billing_contact_by_company;
DROP INDEX IF EXISTS rel_billing_contact_by_contact;
