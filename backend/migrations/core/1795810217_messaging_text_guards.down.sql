-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."chat_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"card_fingerprint":120,"card_message_id":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_fingerprint":120,"card_message_id":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."chat_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"card_fingerprint":120,"card_message_id":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_fingerprint":120,"card_message_id":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."chat_post_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"event_type":20000,"message_id":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"event_type":20000,"message_id":500}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."chat_post_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"event_type":20000,"message_id":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"event_type":20000,"message_id":500}');
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."chat_bot_delivery";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."chat_bot_delivery";
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."chat_channel_watch";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."chat_channel_watch";
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."chat_member_grant";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."chat_member_grant";
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."chat_member_room";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."chat_member_room";
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."chat_root";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."chat_root";
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."chat_send_attempt";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."chat_send_attempt";
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."chat_team_installation";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."chat_team_installation";
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."chat_team_thread";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."chat_team_thread";
RESET lock_timeout;
