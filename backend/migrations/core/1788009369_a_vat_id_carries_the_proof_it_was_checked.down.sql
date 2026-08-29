-- Rolling back discards the consultation numbers, and those are the one thing
-- here that cannot be recreated: VIES issues a receipt for the moment it was
-- asked, and asking again tomorrow proves nothing about a supply invoiced
-- today. So this refuses while any receipt is held, rather than dropping the
-- evidence a business would show for an intra-community supply.
-- Bounded, because the count below reads a table live writers use: without a
-- ceiling an open transaction holding a conflicting lock would stall every
-- write to it for as long as this rollback is willing to queue.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE held bigint;
BEGIN
    -- Writers are locked OUT before the count, not merely counted around. A
    -- plain SELECT takes ACCESS SHARE, which a concurrent insert's ROW
    -- EXCLUSIVE is compatible with: a receipt committed between the count and
    -- the drop would be discarded by a rollback that reported nothing to lose.
    LOCK TABLE organization_vat_check IN ACCESS EXCLUSIVE MODE;
    SELECT count(*) INTO held FROM organization_vat_check
     WHERE consultation_number IS NOT NULL;
    IF held > 0 THEN
        RAISE EXCEPTION 'refusing to roll back: % VAT check(s) hold a VIES consultation number', held
        USING HINT = 'those receipts cannot be re-obtained for the date they were issued — delete the rows first if the loss is intended';
    END IF;
END $$;

DROP TABLE organization_vat_check;
