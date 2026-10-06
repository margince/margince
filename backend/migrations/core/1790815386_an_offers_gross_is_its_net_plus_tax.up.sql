-- offer stores net, tax and gross, and nothing held the arithmetic between them.
--
-- The three are DERIVED: summed in Go from offer_line_item and written back as
-- three independent values. offer_line_item is itself well constrained and stores
-- no line total, so the lines cannot drift against themselves — all the risk is
-- in that one write-back. A recompute that is missed, partial, or racing another
-- leaves the offer carrying three numbers that disagree with each other and with
-- its own lines, and the first reader to notice is the buyer looking at a PDF
-- whose total is not its subtotal plus its tax.
--
-- Enforce arithmetic on money we COMPUTE, never on money we COPY. That is why
-- finance_invoice is left alone: it carries the same three columns, mirrored from
-- an external finance system under that system's rounding, keyed
-- UNIQUE (connection_id, external_id) with sync_hash recording what was copied. A
-- CHECK refusing to store what the source actually says would turn a
-- reconciliation problem into a sync failure.
--
-- NOT VALID, and the scan is a file of its own. offer is a written table on a
-- deployed install, and a plain ADD CONSTRAINT would check every existing row
-- WHILE HOLDING ACCESS EXCLUSIVE — lock_timeout bounds how long a statement waits
-- to acquire a lock, never how long it holds one. NOT VALID takes the lock only
-- long enough to record the constraint.
--
-- Unlike the widenings in this tree, this scan CAN fail: a drifted row is the
-- defect the constraint exists to stop, so one may already be stored. The next
-- migration repairs those rows before it validates.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer
    ADD CONSTRAINT offer_gross_is_net_plus_tax
    CHECK (gross_minor = net_minor + tax_minor) NOT VALID;
