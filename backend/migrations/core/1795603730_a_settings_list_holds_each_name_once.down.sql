-- Reverses 1795603730. A twin the up step renamed keeps its new name.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS deal_acquisition_source_label_once;
DROP INDEX IF EXISTS deal_loss_reason_label_once;
DROP INDEX IF EXISTS partner_stage_label_once;
DROP INDEX IF EXISTS record_role_label_once;
DROP INDEX IF EXISTS lead_origin_label_once;
DROP INDEX IF EXISTS stage_name_once;
DROP INDEX IF EXISTS knowledge_corpus_name_once;

DROP INDEX team_name_unique;
CREATE UNIQUE INDEX team_name_unique ON team (name) WHERE archived_at IS NULL;

DROP INDEX pipeline_name_unique;
CREATE UNIQUE INDEX pipeline_name_unique ON pipeline (name) WHERE archived_at IS NULL;
