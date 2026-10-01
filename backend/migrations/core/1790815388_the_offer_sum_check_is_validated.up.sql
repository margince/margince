-- The second half of the offer money sum, and a file of its own because this
-- runner gives each migration ONE transaction: validating where the constraint
-- was added would run the scan under the ACCESS EXCLUSIVE that ALTER still holds.
-- VALIDATE takes SHARE UPDATE EXCLUSIVE, which readers and writers both pass.
--
-- The repair first, because this scan can fail. gross is recomputed from net and
-- tax rather than from offer_line_item: a row whose gross disagrees with its own
-- net and tax is self-contradictory whatever the lines say, and this is the one
-- repair that needs no pricing arithmetic here. Re-deriving from the lines would
-- put a second implementation of that arithmetic in SQL, which is the thing the
-- Go seam exists to be the only copy of — a row whose NET drifted from its lines
-- is a different defect, and the next recompute of that offer corrects it.
SET LOCAL lock_timeout = '3s';

UPDATE offer SET gross_minor = net_minor + tax_minor
    WHERE gross_minor <> net_minor + tax_minor;

ALTER TABLE offer VALIDATE CONSTRAINT offer_gross_is_net_plus_tax;
