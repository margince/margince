SET LOCAL lock_timeout = '3s';

UPDATE activity_review_template t
   SET questions = (
         SELECT jsonb_agg(q - 'label_key' ORDER BY ord)
           FROM jsonb_array_elements(t.questions) WITH ORDINALITY AS a (q, ord))
 WHERE t.system;

DROP TRIGGER trg_deal_acquisition_source_label_key ON deal_acquisition_source;
DROP TRIGGER trg_lead_origin_label_key ON lead_origin;
DROP TRIGGER trg_lead_disqualify_reason_label_key ON lead_disqualify_reason;
DROP TRIGGER trg_record_role_label_key ON record_role;
DROP TRIGGER trg_activity_review_template_label_key ON activity_review_template;
DROP FUNCTION clear_label_key_on_relabel();

ALTER TABLE deal_acquisition_source DROP COLUMN label_key;
ALTER TABLE lead_origin DROP COLUMN label_key;
ALTER TABLE lead_disqualify_reason DROP COLUMN label_key;
ALTER TABLE record_role DROP COLUMN label_key;
ALTER TABLE activity_review_template DROP COLUMN label_key;
