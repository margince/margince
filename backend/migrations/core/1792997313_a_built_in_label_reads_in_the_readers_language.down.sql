SET LOCAL lock_timeout = '3s';

DROP TRIGGER trg_activity_review_template_question_keys ON activity_review_template;
DROP FUNCTION key_shipped_review_questions();

UPDATE activity_review_template t
   SET questions = (
         SELECT jsonb_agg(q - 'label_key' - 'shipped_label' - 'shipped_label_key' ORDER BY ord)
           FROM jsonb_array_elements(t.questions) WITH ORDINALITY AS a (q, ord))
 WHERE t.system;

ALTER TABLE deal_acquisition_source DROP COLUMN label_key, DROP COLUMN shipped_label, DROP COLUMN shipped_label_key;
ALTER TABLE lead_origin DROP COLUMN label_key, DROP COLUMN shipped_label, DROP COLUMN shipped_label_key;
ALTER TABLE lead_disqualify_reason DROP COLUMN label_key, DROP COLUMN shipped_label, DROP COLUMN shipped_label_key;
ALTER TABLE record_role DROP COLUMN label_key, DROP COLUMN shipped_label, DROP COLUMN shipped_label_key;
ALTER TABLE activity_review_template DROP COLUMN label_key, DROP COLUMN shipped_label, DROP COLUMN shipped_label_key;

DROP FUNCTION label_key_while_shipped(text, text, text);
