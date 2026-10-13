-- A provisional object names the writer that put it, because the key cannot.
--
-- The sweep over this table has to ask the owning module whether a key is still
-- referenced, and it cannot learn which module that is from the key. The intended
-- layout carries it -- blobstore.WorkspaceKey writes <workspace>/<kind>/<id> -- but
-- the offer path writes offers/<workspace>/<id>/<revision>/<uuid>.pdf, where the
-- first segment is the kind and the second the workspace. A sweep parsing position
-- reads one writer's workspace as another writer's kind, and the answer it gets back
-- is "unreferenced" for a live file.
--
-- So the writer declares it. Existing rows are the attachment path's: it is the only
-- writer that has ever recorded an intent, which is the defect the declaration exists
-- to close.
SET LOCAL lock_timeout = '3s';

ALTER TABLE stored_object_intent ADD COLUMN kind text;

UPDATE stored_object_intent SET kind = 'attachment' WHERE kind IS NULL;

-- NOT NULL rather than a DEFAULT. A default would hand a writer that forgot to
-- declare a kind the attachment path's answer, and the sweep would then ask the
-- attachment table about a knowledge document's key and be told nobody references
-- it. The failure has to be a refused insert, not a plausible wrong answer.
ALTER TABLE stored_object_intent ALTER COLUMN kind SET NOT NULL;
