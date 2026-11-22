SET LOCAL lock_timeout = '3s';

ALTER TABLE partner DROP CONSTRAINT partner_source_check;
ALTER TABLE project DROP CONSTRAINT project_source_check;
ALTER TABLE offer DROP CONSTRAINT offer_source_check;
ALTER TABLE deal DROP CONSTRAINT deal_source_check;
