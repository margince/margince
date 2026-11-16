-- Which network a member's pushed message came from, and which client sent it in:
-- a passport, an AI assistant's connection, or the member's own session.
SET LOCAL lock_timeout = '5s';

CREATE TABLE personal_channel_message (
    activity_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    network text NOT NULL,
    origin_kind text NOT NULL,
    origin_id uuid,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT personal_channel_message_pkey PRIMARY KEY (activity_id),
    CONSTRAINT personal_channel_message_activity_id_fkey FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE,
    CONSTRAINT personal_channel_message_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES personal_channel_connection(id) ON DELETE CASCADE,
    CONSTRAINT personal_channel_message_origin_check CHECK (origin_kind IN ('passport', 'connection', 'session')),
    CONSTRAINT personal_channel_message_origin_id_check CHECK ((origin_kind = 'session') = (origin_id IS NULL))
);

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE personal_channel_message TO margince_app;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."personal_channel_message" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"network":120,"origin_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"network":120,"origin_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."personal_channel_message" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"network":120,"origin_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"network":120,"origin_kind":20000}');
