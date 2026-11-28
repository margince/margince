-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity" FOR EACH ROW
WHEN (char_length(NEW."agenda_fingerprint") > 20000
 OR char_length(NEW."audience") > 20000
 OR char_length(NEW."audience_reason") > 2000
 OR char_length(NEW."body_completeness") > 20000
 OR char_length(NEW."calendar_method") > 20000
 OR char_length(NEW."calendar_uid") > 20000
 OR char_length(NEW."call_outcome") > 20000
 OR char_length(NEW."capture_label") > 120
 OR char_length(NEW."capture_label_declined_ruleset") > 20000
 OR char_length(NEW."captured_by") > 20000
 OR char_length(NEW."channel_provider") > 20000
 OR char_length(NEW."counterparty_email") > 20000
 OR char_length(NEW."deal_topic_digest") > 20000
 OR char_length(NEW."direction") > 20000
 OR char_length(NEW."due_zone") > 20000
 OR char_length(NEW."kind") > 20000
 OR char_length(NEW."language") > 20000
 OR char_length(NEW."meeting_notes") > 20000
 OR char_length(NEW."meeting_status") > 20000
 OR char_length(NEW."meeting_zone") > 20000
 OR char_length(NEW."occurred_precision") > 20000
 OR char_length(NEW."origin") > 20000
 OR char_length(NEW."owed_ask") > 2000
 OR char_length(NEW."owed_verdict") > 20000
 OR char_length(NEW."owed_verdict_declined_ruleset") > 20000
 OR char_length(NEW."owed_verdict_ruleset") > 20000
 OR public.text_values_exceed_bound(NEW."redacted_fields", NULL, 20000)
 OR char_length(NEW."remind_zone") > 20000
 OR char_length(NEW."reply_verdict") > 20000
 OR char_length(NEW."reply_verdict_by") > 20000
 OR char_length(NEW."restricted_reason") > 2000
 OR char_length(NEW."retention_class") > 20000
 OR char_length(NEW."source") > 20000
 OR char_length(NEW."source_id") > 20000
 OR char_length(NEW."source_system") > 20000
 OR char_length(NEW."subject") > 20000
 OR char_length(NEW."task_type") > 20000
 OR char_length(NEW."thread_key") > 20000) EXECUTE FUNCTION public.refuse_oversized_text('{"agenda_fingerprint":20000,"audience":20000,"audience_reason":2000,"body_completeness":20000,"calendar_method":20000,"calendar_uid":20000,"call_outcome":20000,"capture_label":120,"capture_label_declined_ruleset":20000,"captured_by":20000,"channel_provider":20000,"counterparty_email":20000,"deal_topic_digest":20000,"direction":20000,"due_zone":20000,"kind":20000,"language":20000,"meeting_notes":20000,"meeting_status":20000,"meeting_zone":20000,"occurred_precision":20000,"origin":20000,"owed_ask":2000,"owed_verdict":20000,"owed_verdict_declined_ruleset":20000,"owed_verdict_ruleset":20000,"redacted_fields":20000,"remind_zone":20000,"reply_verdict":20000,"reply_verdict_by":20000,"restricted_reason":2000,"retention_class":20000,"source":20000,"source_id":20000,"source_system":20000,"subject":20000,"task_type":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity" FOR EACH ROW
WHEN ((char_length(NEW."agenda_fingerprint") > 20000 AND NEW."agenda_fingerprint" COLLATE "C" IS DISTINCT FROM OLD."agenda_fingerprint" COLLATE "C")
 OR (char_length(NEW."audience") > 20000 AND NEW."audience" COLLATE "C" IS DISTINCT FROM OLD."audience" COLLATE "C")
 OR (char_length(NEW."audience_reason") > 2000 AND NEW."audience_reason" COLLATE "C" IS DISTINCT FROM OLD."audience_reason" COLLATE "C")
 OR (char_length(NEW."body_completeness") > 20000 AND NEW."body_completeness" COLLATE "C" IS DISTINCT FROM OLD."body_completeness" COLLATE "C")
 OR (char_length(NEW."calendar_method") > 20000 AND NEW."calendar_method" COLLATE "C" IS DISTINCT FROM OLD."calendar_method" COLLATE "C")
 OR (char_length(NEW."calendar_uid") > 20000 AND NEW."calendar_uid" COLLATE "C" IS DISTINCT FROM OLD."calendar_uid" COLLATE "C")
 OR (char_length(NEW."call_outcome") > 20000 AND NEW."call_outcome" COLLATE "C" IS DISTINCT FROM OLD."call_outcome" COLLATE "C")
 OR (char_length(NEW."capture_label") > 120 AND NEW."capture_label" COLLATE "C" IS DISTINCT FROM OLD."capture_label" COLLATE "C")
 OR (char_length(NEW."capture_label_declined_ruleset") > 20000 AND NEW."capture_label_declined_ruleset" COLLATE "C" IS DISTINCT FROM OLD."capture_label_declined_ruleset" COLLATE "C")
 OR (char_length(NEW."captured_by") > 20000 AND NEW."captured_by" COLLATE "C" IS DISTINCT FROM OLD."captured_by" COLLATE "C")
 OR (char_length(NEW."channel_provider") > 20000 AND NEW."channel_provider" COLLATE "C" IS DISTINCT FROM OLD."channel_provider" COLLATE "C")
 OR (char_length(NEW."counterparty_email") > 20000 AND NEW."counterparty_email" COLLATE "C" IS DISTINCT FROM OLD."counterparty_email" COLLATE "C")
 OR (char_length(NEW."deal_topic_digest") > 20000 AND NEW."deal_topic_digest" COLLATE "C" IS DISTINCT FROM OLD."deal_topic_digest" COLLATE "C")
 OR (char_length(NEW."direction") > 20000 AND NEW."direction" COLLATE "C" IS DISTINCT FROM OLD."direction" COLLATE "C")
 OR (char_length(NEW."due_zone") > 20000 AND NEW."due_zone" COLLATE "C" IS DISTINCT FROM OLD."due_zone" COLLATE "C")
 OR (char_length(NEW."kind") > 20000 AND NEW."kind" COLLATE "C" IS DISTINCT FROM OLD."kind" COLLATE "C")
 OR (char_length(NEW."language") > 20000 AND NEW."language" COLLATE "C" IS DISTINCT FROM OLD."language" COLLATE "C")
 OR (char_length(NEW."meeting_notes") > 20000 AND NEW."meeting_notes" COLLATE "C" IS DISTINCT FROM OLD."meeting_notes" COLLATE "C")
 OR (char_length(NEW."meeting_status") > 20000 AND NEW."meeting_status" COLLATE "C" IS DISTINCT FROM OLD."meeting_status" COLLATE "C")
 OR (char_length(NEW."meeting_zone") > 20000 AND NEW."meeting_zone" COLLATE "C" IS DISTINCT FROM OLD."meeting_zone" COLLATE "C")
 OR (char_length(NEW."occurred_precision") > 20000 AND NEW."occurred_precision" COLLATE "C" IS DISTINCT FROM OLD."occurred_precision" COLLATE "C")
 OR (char_length(NEW."origin") > 20000 AND NEW."origin" COLLATE "C" IS DISTINCT FROM OLD."origin" COLLATE "C")
 OR (char_length(NEW."owed_ask") > 2000 AND NEW."owed_ask" COLLATE "C" IS DISTINCT FROM OLD."owed_ask" COLLATE "C")
 OR (char_length(NEW."owed_verdict") > 20000 AND NEW."owed_verdict" COLLATE "C" IS DISTINCT FROM OLD."owed_verdict" COLLATE "C")
 OR (char_length(NEW."owed_verdict_declined_ruleset") > 20000 AND NEW."owed_verdict_declined_ruleset" COLLATE "C" IS DISTINCT FROM OLD."owed_verdict_declined_ruleset" COLLATE "C")
 OR (char_length(NEW."owed_verdict_ruleset") > 20000 AND NEW."owed_verdict_ruleset" COLLATE "C" IS DISTINCT FROM OLD."owed_verdict_ruleset" COLLATE "C")
 OR (NEW."redacted_fields" IS DISTINCT FROM OLD."redacted_fields" AND public.text_values_exceed_bound(NEW."redacted_fields", OLD."redacted_fields", 20000))
 OR (char_length(NEW."remind_zone") > 20000 AND NEW."remind_zone" COLLATE "C" IS DISTINCT FROM OLD."remind_zone" COLLATE "C")
 OR (char_length(NEW."reply_verdict") > 20000 AND NEW."reply_verdict" COLLATE "C" IS DISTINCT FROM OLD."reply_verdict" COLLATE "C")
 OR (char_length(NEW."reply_verdict_by") > 20000 AND NEW."reply_verdict_by" COLLATE "C" IS DISTINCT FROM OLD."reply_verdict_by" COLLATE "C")
 OR (char_length(NEW."restricted_reason") > 2000 AND NEW."restricted_reason" COLLATE "C" IS DISTINCT FROM OLD."restricted_reason" COLLATE "C")
 OR (char_length(NEW."retention_class") > 20000 AND NEW."retention_class" COLLATE "C" IS DISTINCT FROM OLD."retention_class" COLLATE "C")
 OR (char_length(NEW."source") > 20000 AND NEW."source" COLLATE "C" IS DISTINCT FROM OLD."source" COLLATE "C")
 OR (char_length(NEW."source_id") > 20000 AND NEW."source_id" COLLATE "C" IS DISTINCT FROM OLD."source_id" COLLATE "C")
 OR (char_length(NEW."source_system") > 20000 AND NEW."source_system" COLLATE "C" IS DISTINCT FROM OLD."source_system" COLLATE "C")
 OR (char_length(NEW."subject") > 20000 AND NEW."subject" COLLATE "C" IS DISTINCT FROM OLD."subject" COLLATE "C")
 OR (char_length(NEW."task_type") > 20000 AND NEW."task_type" COLLATE "C" IS DISTINCT FROM OLD."task_type" COLLATE "C")
 OR (char_length(NEW."thread_key") > 20000 AND NEW."thread_key" COLLATE "C" IS DISTINCT FROM OLD."thread_key" COLLATE "C")) EXECUTE FUNCTION public.refuse_oversized_text('{"agenda_fingerprint":20000,"audience":20000,"audience_reason":2000,"body_completeness":20000,"calendar_method":20000,"calendar_uid":20000,"call_outcome":20000,"capture_label":120,"capture_label_declined_ruleset":20000,"captured_by":20000,"channel_provider":20000,"counterparty_email":20000,"deal_topic_digest":20000,"direction":20000,"due_zone":20000,"kind":20000,"language":20000,"meeting_notes":20000,"meeting_status":20000,"meeting_zone":20000,"occurred_precision":20000,"origin":20000,"owed_ask":2000,"owed_verdict":20000,"owed_verdict_declined_ruleset":20000,"owed_verdict_ruleset":20000,"redacted_fields":20000,"remind_zone":20000,"reply_verdict":20000,"reply_verdict_by":20000,"restricted_reason":2000,"retention_class":20000,"source":20000,"source_id":20000,"source_system":20000,"subject":20000,"task_type":20000,"thread_key":20000}');
RESET lock_timeout;
