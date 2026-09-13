-- Recover the notification projection from its exact event causation chain.
-- Never match deal names or nearby timestamps: neither identifies a move.
-- This repairs delivered text and metadata, retaining the notice ID/read state;
-- it does not replay a stage move or deliver another notification.
SET LOCAL lock_timeout = '3s';
WITH delivered AS (
  SELECT envelope->'payload'->>'notice_id' AS notice_id,
         envelope->'trace'->>'causation_id' AS cause
    FROM event_outbox
   WHERE envelope->>'type' = 'notice.created'
), candidates AS (
  SELECT n.id, e.envelope,
         CASE WHEN n.subject = 'A deal you own changed stage'
              THEN left(n.body, length(n.body) - length(' moved to a new pipeline stage.'))
              ELSE left(n.subject, length(n.subject) - length(' changed stage')) END AS deal_name,
         coalesce(nullif(e.envelope->'payload'->>'from_stage_name', ''),
           CASE WHEN f.updated_at <= (e.envelope->>'occurred_at')::timestamptz THEN f.name END) AS from_name,
         coalesce(nullif(e.envelope->'payload'->>'to_stage_name', ''),
           CASE WHEN t.updated_at <= (e.envelope->>'occurred_at')::timestamptz THEN t.name END) AS to_name,
         count(*) OVER (PARTITION BY n.id) AS matches
    FROM notice n
    JOIN delivered d ON d.notice_id = n.id::text
    JOIN event_outbox e ON e.envelope->>'event_id' = d.cause
    LEFT JOIN stage f ON f.id::text = e.envelope->'payload'->>'from_stage_id'
    LEFT JOIN stage t ON t.id::text = e.envelope->'payload'->>'to_stage_id'
   WHERE n.kind = 'automation'
     AND (n.origin IS NULL OR NOT n.origin ? 'stage_change')
     AND e.envelope->>'type' = 'deal.stage_changed'
     AND e.envelope->'entity'->>'type' = 'deal'
     AND (n.target_id IS NULL OR n.target_id::text = e.envelope->'entity'->>'id')
     AND (n.origin IS NULL OR n.origin->>'event_id' = e.envelope->>'event_id')
     AND ((n.subject = 'A deal you own changed stage'
           AND n.body LIKE '% moved to a new pipeline stage.')
          OR (n.subject LIKE '% changed stage'
              AND (n.body LIKE '% moved to a new pipeline stage.' OR n.body LIKE 'Stage changed: %')))
)
UPDATE notice n
   SET subject = c.deal_name,
       body = coalesce(c.from_name, 'Unknown stage') || ' → ' || coalesce(c.to_name, 'Unknown stage'),
       target_type = 'deal',
       target_id = (c.envelope->'entity'->>'id')::uuid,
       origin = jsonb_strip_nulls(jsonb_build_object(
         'event_id', c.envelope->>'event_id',
         'actor_type', c.envelope->'actor'->>'type',
         'actor_id', c.envelope->'actor'->>'id',
         'on_behalf_of', c.envelope->'actor'->'on_behalf_of',
         'occurred_at', c.envelope->>'occurred_at',
         'stage_change', jsonb_build_object('from_name', c.from_name, 'to_name', c.to_name)))
  FROM candidates c
 WHERE n.id = c.id AND c.matches = 1 AND c.deal_name <> '';
