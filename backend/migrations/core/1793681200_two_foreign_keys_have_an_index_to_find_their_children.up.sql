SET LOCAL lock_timeout = '3s';

-- Deleting an offer line sets the allocation's reference to it null, and finds
-- the allocations through this index.
CREATE INDEX deal_product_offer_line_item_idx ON deal_product (offer_line_item_id)
    WHERE offer_line_item_id IS NOT NULL;

-- Deleting a deal removes its initiative baselines, and finds them through this
-- index. The primary key leads with the initiative, so it cannot.
CREATE INDEX sales_initiative_baseline_deal_idx ON sales_initiative_baseline (deal_id);
