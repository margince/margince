-- A built-in list value or review question names the catalog key a screen
-- renders it from, in the reader's language. The stored label stays the
-- English text that an export, a report or an agent reads.
--
-- A built-in row keeps the wording and key it shipped with. It names its key
-- only while its label reads as shipped: an administrator's own words win, and
-- putting the shipped words back makes it read translated again.
SET LOCAL lock_timeout = '3s';

-- The one rule every built-in label follows, for the list columns and the
-- review questions alike.
CREATE FUNCTION label_key_while_shipped(label text, shipped_label text, shipped_label_key text)
    RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$ SELECT CASE WHEN label = shipped_label THEN shipped_label_key END $$;

ALTER TABLE deal_acquisition_source ADD COLUMN shipped_label text, ADD COLUMN shipped_label_key text;
ALTER TABLE lead_origin ADD COLUMN shipped_label text, ADD COLUMN shipped_label_key text;
ALTER TABLE lead_disqualify_reason ADD COLUMN shipped_label text, ADD COLUMN shipped_label_key text;
ALTER TABLE record_role ADD COLUMN shipped_label text, ADD COLUMN shipped_label_key text;
ALTER TABLE activity_review_template ADD COLUMN shipped_label text, ADD COLUMN shipped_label_key text;

-- Matched by key, so a value an administrator already renamed keeps the name
-- they chose and still reads translated if they put the shipped words back.
UPDATE deal_acquisition_source s
   SET shipped_label = shipped.label, shipped_label_key = 'acqSources.builtin.' || s.key
  FROM (VALUES
    ('inbound', 'Inbound'),
    ('outbound', 'Outbound'),
    ('referral', 'Referral'),
    ('partner', 'Partner'),
    ('event', 'Event'),
    ('existing_customer', 'Existing customer'),
    ('employee_referral', 'Employee referral'),
    ('other', 'Other')
  ) AS shipped (key, label)
 WHERE s.system AND s.key = shipped.key;

UPDATE lead_origin o
   SET shipped_label = shipped.label, shipped_label_key = 'leadOrigins.builtin.' || o.key
  FROM (VALUES
    ('event', 'Event'),
    ('linkedin', 'LinkedIn'),
    ('referral', 'Referral'),
    ('partner', 'Partner'),
    ('website', 'Website'),
    ('cold_outreach', 'Cold outreach'),
    ('other', 'Other')
  ) AS shipped (key, label)
 WHERE o.system AND o.key = shipped.key;

-- A reason row has no key of its own, so only a row still worded as shipped
-- can be told apart.
UPDATE lead_disqualify_reason r
   SET shipped_label = shipped.label, shipped_label_key = 'leadReasons.builtin.' || shipped.slug
  FROM (VALUES
    ('not_a_good_fit', 'Not a good fit'),
    ('bad_timing', 'Bad timing'),
    ('no_budget', 'No budget'),
    ('no_decision_power', 'No decision power'),
    ('chose_a_competitor', 'Chose a competitor'),
    ('no_interest', 'No interest'),
    ('not_reachable', 'Not reachable'),
    ('duplicate_or_spam', 'Duplicate or spam')
  ) AS shipped (slug, label)
 WHERE r.system AND r.label = shipped.label;

UPDATE record_role r
   SET shipped_label = shipped.label, shipped_label_key = 'recordRoles.builtin.' || r.key
  FROM (VALUES
    ('account_manager', 'Account manager'),
    ('sales_engineer', 'Sales engineer'),
    ('delivery_lead', 'Delivery lead'),
    ('project_manager', 'Project manager'),
    ('technical_contact', 'Technical contact'),
    ('executive_sponsor', 'Executive sponsor'),
    ('support_team', 'Support team')
  ) AS shipped (key, label)
 WHERE r.system AND r.key = shipped.key;

UPDATE activity_review_template t
   SET shipped_label = shipped.label, shipped_label_key = 'reviewTemplates.builtin.' || t.key
  FROM (VALUES
    ('win_review', 'Win review'),
    ('loss_review', 'Loss review')
  ) AS shipped (key, label)
 WHERE t.system AND t.key = shipped.key;

ALTER TABLE deal_acquisition_source ADD COLUMN label_key text
    GENERATED ALWAYS AS (label_key_while_shipped(label, shipped_label, shipped_label_key)) STORED;
ALTER TABLE lead_origin ADD COLUMN label_key text
    GENERATED ALWAYS AS (label_key_while_shipped(label, shipped_label, shipped_label_key)) STORED;
ALTER TABLE lead_disqualify_reason ADD COLUMN label_key text
    GENERATED ALWAYS AS (label_key_while_shipped(label, shipped_label, shipped_label_key)) STORED;
ALTER TABLE record_role ADD COLUMN label_key text
    GENERATED ALWAYS AS (label_key_while_shipped(label, shipped_label, shipped_label_key)) STORED;
ALTER TABLE activity_review_template ADD COLUMN label_key text
    GENERATED ALWAYS AS (label_key_while_shipped(label, shipped_label, shipped_label_key)) STORED;

-- A question is a JSON object, so its key goes inside it, set on every save
-- of a built-in template from the wording each question shipped with. A writer
-- cannot supply the key, and a question deleted and added back gets it again.
-- A review copies the question whole, so a later review keeps the key.
CREATE FUNCTION key_shipped_review_questions() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.questions = (
    SELECT jsonb_agg(
             (q - 'label_key')
             || jsonb_strip_nulls(jsonb_build_object('label_key', label_key_while_shipped(
                  q ->> 'label', asked.label, 'reviewTemplates.builtin.question.' || asked.key)))
             ORDER BY ord)
      FROM jsonb_array_elements(NEW.questions) WITH ORDINALITY AS a (q, ord)
      LEFT JOIN (VALUES
        ('win_review', 'why_we_won', 'Why did we win?'),
        ('win_review', 'competitors', 'Who else were they considering?'),
        ('win_review', 'decisive_factor', 'What decided it?'),
        ('loss_review', 'why_we_lost', 'Why did we lose?'),
        ('loss_review', 'lost_to', 'Who did they choose instead?'),
        ('loss_review', 'what_would_change_it', 'What would have changed the outcome?')
      ) AS asked (template, key, label)
        ON NEW.system AND asked.template = NEW.key AND asked.key = q ->> 'key');
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_activity_review_template_question_keys
    BEFORE INSERT OR UPDATE OF questions ON activity_review_template
    FOR EACH ROW EXECUTE FUNCTION key_shipped_review_questions();

UPDATE activity_review_template SET questions = questions WHERE system;
