SET LOCAL lock_timeout = '5s';

ALTER TABLE weekly_review
    DROP CONSTRAINT IF EXISTS weekly_review_prior_fkey,
    DROP CONSTRAINT IF EXISTS weekly_review_currency_check,
    DROP CONSTRAINT IF EXISTS weekly_review_money_names_its_currency,
    DROP CONSTRAINT IF EXISTS weekly_review_next_steps_were_meetings,
    DROP CONSTRAINT IF EXISTS weekly_review_new_counts_are_tallies,
    DROP COLUMN IF EXISTS prior_review_id,
    DROP COLUMN IF EXISTS base_currency,
    DROP COLUMN IF EXISTS pipeline_lost_minor,
    DROP COLUMN IF EXISTS pipeline_won_minor,
    DROP COLUMN IF EXISTS pipeline_created_minor,
    DROP COLUMN IF EXISTS meetings_with_next_step,
    DROP COLUMN IF EXISTS meetings_held,
    DROP COLUMN IF EXISTS leads_breached,
    DROP COLUMN IF EXISTS leads_answered_in_target,
    DROP COLUMN IF EXISTS leads_routed;
