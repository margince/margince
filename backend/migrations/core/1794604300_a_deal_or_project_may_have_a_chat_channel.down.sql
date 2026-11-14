-- Reverses the chat channel links and the Dispact bot connection.
SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS chat_post_item;
DROP TABLE IF EXISTS chat_link;

DELETE FROM channel_connection WHERE provider = 'dispact';
DELETE FROM channel_provider WHERE provider = 'dispact';

ALTER TABLE channel_connection DROP CONSTRAINT IF EXISTS channel_connection_address_check;
ALTER TABLE channel_connection DROP COLUMN IF EXISTS events_credential_ref;
ALTER TABLE channel_connection DROP COLUMN IF EXISTS address;
ALTER TABLE channel_connection DROP CONSTRAINT channel_connection_provider_check;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_provider_check CHECK (provider = 'telegram') NOT VALID;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"channel_id":20000,"channel_label":120,"credential_ref":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel_id":20000,"channel_label":120,"credential_ref":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"channel_id":20000,"channel_label":120,"credential_ref":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel_id":20000,"channel_label":120,"credential_ref":20000,"provider":20000,"status":20000}');
