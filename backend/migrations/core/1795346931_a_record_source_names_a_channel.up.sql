-- A deal, offer, project or partner row says which channel it arrived by. The
-- checks are added unvalidated; a data task moves older rows onto the list.
SET LOCAL lock_timeout = '3s';

-- A partner row is written from the partner form, so an older value is manual.
UPDATE partner SET source = 'manual'
 WHERE source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform');

ALTER TABLE deal ADD CONSTRAINT deal_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;
ALTER TABLE offer ADD CONSTRAINT offer_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;
ALTER TABLE project ADD CONSTRAINT project_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;
ALTER TABLE partner ADD CONSTRAINT partner_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;
