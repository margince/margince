SET LOCAL lock_timeout = '3s';
UPDATE saved_view
   SET query = jsonb_set(query, '{list}',
         (query->'list') - 'sort'
         || CASE WHEN query->'list' ? 'sort'
                 THEN jsonb_build_object('sort', regexp_replace(query->'list'->>'sort', '^(-?)status$', '\1lifecycle'))
                 ELSE '{}'::jsonb END
         || CASE WHEN jsonb_typeof(query->'list'->'filters') = 'object' AND query->'list'->'filters' ? 'status'
                 THEN jsonb_build_object('filters', (query->'list'->'filters') - 'status'
                        || jsonb_build_object('lifecycle', query->'list'->'filters'->'status'))
                 ELSE '{}'::jsonb END)
 WHERE resource = 'companies'
   AND jsonb_typeof(query->'list') = 'object'
   AND (query->'list'->>'sort' IN ('status', '-status')
        OR (jsonb_typeof(query->'list'->'filters') = 'object' AND query->'list'->'filters' ? 'status'));

ALTER TABLE site_read RENAME COLUMN logo_icon_source TO logo_icon_origin;
ALTER TABLE site_read RENAME COLUMN logo_source TO logo_origin;

ALTER INDEX idx_company_status RENAME TO idx_company_lifecycle;
ALTER TABLE company RENAME CONSTRAINT company_status_check TO company_lifecycle_check;
ALTER TABLE company RENAME CONSTRAINT company_logo_icon_names_its_source TO company_logo_icon_names_its_origin;
ALTER TABLE company RENAME CONSTRAINT company_logo_names_its_source TO company_logo_names_its_origin;
ALTER TABLE company RENAME COLUMN status TO lifecycle;
ALTER TABLE company RENAME COLUMN geocode_source TO geocode_provider;
ALTER TABLE company RENAME COLUMN logo_icon_source TO logo_icon_origin;
ALTER TABLE company RENAME COLUMN logo_source TO logo_origin;
