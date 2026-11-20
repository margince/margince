SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_link
    DROP CONSTRAINT activity_link_deal_relevance_check,
    ADD CONSTRAINT activity_link_deal_relevance_check
        CHECK (deal_relevance = ANY (ARRAY['relevant', 'unrelated', 'uncertain'])) NOT VALID,
    DROP CONSTRAINT activity_link_relevance_source_check,
    ADD CONSTRAINT activity_link_relevance_source_check
        CHECK (relevance_source = ANY (ARRAY['human', 'classifier'])) NOT VALID,
    DROP CONSTRAINT activity_link_deal_relevance_shape,
    ADD CONSTRAINT activity_link_deal_relevance_shape
        CHECK ((deal_relevance IS NULL) = (relevance_source IS NULL)
               AND (deal_relevance IS NULL OR entity_type = 'deal')) NOT VALID;
