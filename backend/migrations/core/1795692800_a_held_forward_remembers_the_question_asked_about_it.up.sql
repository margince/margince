SET LOCAL lock_timeout = '3s';

-- A message a colleague forwarded to a company chat account, whose author the
-- network did not name, waits for the colleague to say who wrote it. The
-- company account asks once per batch of such forwards. These columns sit on the
-- stored original of the held message, so the question, the batch and the held
-- message leave together when the forward expires.
--
--   held_by            the colleague who forwarded the message.
--   held_connection    the company account's channel_connection the message came
--                      through; empty for the workspace's own bot.
--   ask_batch_id       the forwards of one colleague on one account within a
--                      short window; empty until the batch has settled.
--   ask_message_ids    the provider ids of every question this system sent about
--                      the message, so the provider's echo of one is never filed.
--   ask_claimed_at     when a send was started. It is written before the call, so
--                      a send whose outcome is unknown is never repeated.
--   ask_sent_at        when the provider accepted the clarification.
--   ask_attempts and ask_next_at pace the retry of a send the provider refused
--                      without delivering.
--   ask_followup_at    a reply that named no batch could not be bound, and the
--                      colleague is owed one question that lists the batches.
ALTER TABLE raw_capture
    ADD COLUMN held_by uuid,
    ADD COLUMN held_connection uuid,
    ADD COLUMN ask_batch_id uuid,
    ADD COLUMN ask_message_ids text[] NOT NULL DEFAULT '{}',
    ADD COLUMN ask_claimed_at timestamptz,
    ADD COLUMN ask_sent_at timestamptz,
    ADD COLUMN ask_attempts smallint NOT NULL DEFAULT 0,
    ADD COLUMN ask_next_at timestamptz,
    ADD COLUMN ask_followup_at timestamptz;
