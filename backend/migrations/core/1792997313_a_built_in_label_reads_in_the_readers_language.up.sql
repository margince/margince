-- A built-in list value or review question names the catalog key a screen
-- renders it from, in the reader's language. The stored label stays the
-- English text that an export, a report or an agent reads.
--
-- Only rows still worded as shipped get a key. An administrator who already
-- renamed one keeps the name they chose.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_acquisition_source ADD COLUMN label_key text;
ALTER TABLE lead_origin ADD COLUMN label_key text;
ALTER TABLE lead_disqualify_reason ADD COLUMN label_key text;
ALTER TABLE record_role ADD COLUMN label_key text;
ALTER TABLE activity_review_template ADD COLUMN label_key text;

-- A relabel is an administrator's own words, so it drops the key. Held here
-- rather than in each writer, so no write path can keep a stale key.
CREATE FUNCTION clear_label_key_on_relabel() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.label IS DISTINCT FROM OLD.label THEN
    NEW.label_key = NULL;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_deal_acquisition_source_label_key
    BEFORE UPDATE OF label ON deal_acquisition_source
    FOR EACH ROW EXECUTE FUNCTION clear_label_key_on_relabel();
CREATE TRIGGER trg_lead_origin_label_key
    BEFORE UPDATE OF label ON lead_origin
    FOR EACH ROW EXECUTE FUNCTION clear_label_key_on_relabel();
CREATE TRIGGER trg_lead_disqualify_reason_label_key
    BEFORE UPDATE OF label ON lead_disqualify_reason
    FOR EACH ROW EXECUTE FUNCTION clear_label_key_on_relabel();
CREATE TRIGGER trg_record_role_label_key
    BEFORE UPDATE OF label ON record_role
    FOR EACH ROW EXECUTE FUNCTION clear_label_key_on_relabel();
CREATE TRIGGER trg_activity_review_template_label_key
    BEFORE UPDATE OF label ON activity_review_template
    FOR EACH ROW EXECUTE FUNCTION clear_label_key_on_relabel();

UPDATE deal_acquisition_source s
   SET label_key = 'acqSources.builtin.' || s.key
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
 WHERE s.system AND s.key = shipped.key AND s.label = shipped.label;

UPDATE lead_origin o
   SET label_key = 'leadOrigins.builtin.' || o.key
  FROM (VALUES
    ('event', 'Event'),
    ('linkedin', 'LinkedIn'),
    ('referral', 'Referral'),
    ('partner', 'Partner'),
    ('website', 'Website'),
    ('cold_outreach', 'Cold outreach'),
    ('other', 'Other')
  ) AS shipped (key, label)
 WHERE o.system AND o.key = shipped.key AND o.label = shipped.label;

-- A reason row has no key of its own, so the label it shipped with names it.
UPDATE lead_disqualify_reason r
   SET label_key = 'leadReasons.builtin.' || shipped.slug
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
   SET label_key = 'recordRoles.builtin.' || r.key
  FROM (VALUES
    ('account_manager', 'Account manager'),
    ('sales_engineer', 'Sales engineer'),
    ('delivery_lead', 'Delivery lead'),
    ('project_manager', 'Project manager'),
    ('technical_contact', 'Technical contact'),
    ('executive_sponsor', 'Executive sponsor'),
    ('support_team', 'Support team')
  ) AS shipped (key, label)
 WHERE r.system AND r.key = shipped.key AND r.label = shipped.label;

-- A question is a JSON object, so its key goes inside it. A response copies
-- the question whole, so a review written from now on keeps the key too.
UPDATE activity_review_template t
   SET label_key = CASE WHEN t.label = shipped.label
                        THEN 'reviewTemplates.builtin.' || t.key END,
       questions = (
         SELECT jsonb_agg(
                  CASE WHEN asked.label = q ->> 'label'
                       THEN q || jsonb_build_object(
                              'label_key', 'reviewTemplates.builtin.question.' || asked.key)
                       ELSE q END
                  ORDER BY ord)
           FROM jsonb_array_elements(t.questions) WITH ORDINALITY AS a (q, ord)
           LEFT JOIN (VALUES
             ('win_review', 'why_we_won', 'Why did we win?'),
             ('win_review', 'competitors', 'Who else were they considering?'),
             ('win_review', 'decisive_factor', 'What decided it?'),
             ('loss_review', 'why_we_lost', 'Why did we lose?'),
             ('loss_review', 'lost_to', 'Who did they choose instead?'),
             ('loss_review', 'what_would_change_it', 'What would have changed the outcome?')
           ) AS asked (template, key, label)
             ON asked.template = t.key AND asked.key = q ->> 'key')
  FROM (VALUES
    ('win_review', 'Win review'),
    ('loss_review', 'Loss review')
  ) AS shipped (key, label)
 WHERE t.system AND t.key = shipped.key;
