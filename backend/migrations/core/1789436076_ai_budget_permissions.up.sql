-- The allowance is a separate operational decision from routing vendors.
UPDATE role SET permissions = jsonb_set(permissions, '{objects,ai_budget}',
  CASE WHEN key IN ('admin','ops') THEN '{"create":false,"read":true,"update":true,"delete":false}'::jsonb
       WHEN key = 'management' THEN '{"create":false,"read":true,"update":false,"delete":false}'::jsonb
       ELSE '{"create":false,"read":false,"update":false,"delete":false}'::jsonb END)
WHERE is_system AND key IN ('admin','ops','management','manager','rep','read_only') AND permissions ? 'objects' AND NOT (permissions -> 'objects') ? 'ai_budget';
