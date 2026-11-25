SET LOCAL lock_timeout = '3s';

DROP TRIGGER project_source_channel ON project;
DROP TRIGGER offer_source_channel ON offer;
DROP TRIGGER deal_source_channel ON deal;
DROP FUNCTION move_source_onto_its_channel();
DROP FUNCTION record_source_channel(text);
ALTER TABLE partner DROP CONSTRAINT partner_source_check;
ALTER TABLE project DROP CONSTRAINT project_source_check;
ALTER TABLE offer DROP CONSTRAINT offer_source_check;
ALTER TABLE deal DROP CONSTRAINT deal_source_check;
