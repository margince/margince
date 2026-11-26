SET LOCAL lock_timeout = '3s';
-- The schema spells provenance `source` and a funnel position `status`; these
-- columns were the last to say `origin`, `provider` and `lifecycle`.
ALTER TABLE company RENAME COLUMN logo_origin TO logo_source;
ALTER TABLE company RENAME COLUMN logo_icon_origin TO logo_icon_source;
ALTER TABLE company RENAME COLUMN geocode_provider TO geocode_source;
ALTER TABLE company RENAME COLUMN lifecycle TO status;
ALTER TABLE company RENAME CONSTRAINT company_logo_names_its_origin TO company_logo_names_its_source;
ALTER TABLE company RENAME CONSTRAINT company_logo_icon_names_its_origin TO company_logo_icon_names_its_source;
ALTER TABLE company RENAME CONSTRAINT company_lifecycle_check TO company_status_check;
ALTER INDEX idx_company_lifecycle RENAME TO idx_company_status;

ALTER TABLE site_read RENAME COLUMN logo_origin TO logo_source;
ALTER TABLE site_read RENAME COLUMN logo_icon_origin TO logo_icon_source;

-- A saved company list keeps the filter and sort it was saved with.
UPDATE saved_view
   SET query = jsonb_set(query, '{list}',
         (query->'list') - 'sort'
         || CASE WHEN query->'list' ? 'sort'
                 THEN jsonb_build_object('sort', regexp_replace(query->'list'->>'sort', '^(-?)lifecycle$', '\1status'))
                 ELSE '{}'::jsonb END
         || CASE WHEN jsonb_typeof(query->'list'->'filters') = 'object' AND query->'list'->'filters' ? 'lifecycle'
                 THEN jsonb_build_object('filters', (query->'list'->'filters') - 'lifecycle'
                        || jsonb_build_object('status', query->'list'->'filters'->'lifecycle'))
                 ELSE '{}'::jsonb END)
 WHERE resource = 'companies'
   AND jsonb_typeof(query->'list') = 'object'
   AND (query->'list'->>'sort' IN ('lifecycle', '-lifecycle')
        OR (jsonb_typeof(query->'list'->'filters') = 'object' AND query->'list'->'filters' ? 'lifecycle'));
