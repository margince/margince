SET LOCAL lock_timeout = '3s';

-- A rep who presses send twice on one refused message is holding ONE message.
--
-- SCOPED TO send_refused, and only to it. Every other hold reason reaches a row
-- by UPDATE: a scheduled message that missed its window, or whose sender went
-- inactive, was already written as a promise to send at a moment and is later
-- stopped. Two such promises with identical words and different due dates are
-- two real intentions a rep is entitled to hold, and an index spanning every
-- reason would refuse the second one at the moment it was held — turning a
-- message that merely missed its window into a write that fails.
--
-- A refusal hold is the only one INSERTED, and the only one where a second copy
-- is a duplicate rather than a second intention.
--
-- A refused send freezes its message into a held scheduled_send so the review
-- recording that refusal has something a human can resume. Pressing send again
-- — which is the ordinary thing to do after reading a refusal — would otherwise
-- freeze a second copy, and each copy would open its own review. One message
-- would then sit in front of somebody twice, and resuming both would deliver it
-- twice.
--
-- The writer looks for a row it can reuse before inserting. That look is a
-- SELECT followed by an INSERT, so two refusals racing each other both find
-- nothing and both insert. This index is what makes the race lose: the second
-- insert is refused by the database, the writer re-reads, and it finds the row
-- the first one wrote.
--
-- WHAT COUNTS AS THE SAME MESSAGE is every column that decides what would
-- actually be sent and under whose authority:
--
--   payload            the frozen message itself — subject, body, recipients,
--                      attachments, the claimed purpose and its evidence
--   payload_version    a payload this build cannot read is not one it may reuse
--   scheduled_by       whose mailbox it would leave from
--   principal_kind     whether a human or an agent would fire it. An agent send
--                      carries the granting human's user id, so scheduled_by
--                      alone would let a human's held message be reused for an
--                      agent's send: the row would keep reading 'human', the
--                      passport check that guards agent sends would be skipped,
--                      and the message would go out over the human's signature.
--   agent_actor_id     which agent, so one agent cannot inherit another's
--   agent_passport_id  identity or its revocation
--   origin_kind        a reply or an account-started message
--   anchor_activity_id which conversation a reply continues. Two identical
--                      messages on two different threads are two messages: the
--                      anchor and the records it carries are part of the
--                      question the consent engine was asked, and resuming the
--                      wrong one files the message under the wrong record.
--   origin_links       the records an account-started message files under, for
--                      the anchor's reason
--   also_links         the records a REPLY was told to file under beyond its
--                      anchor's. Named at composition and stored rather than
--                      re-derived, because nothing at fire could work out which
--                      ones the rep meant — so two replies differing only here
--                      file under different records and are two messages
--
-- coalesce on the nullable columns because NULL is never equal to NULL in a
-- unique index, so two rows that both have no anchor would not collide on it —
-- and those are exactly the account-started messages this must catch.
--
-- md5 ON THE JSON, not the json itself, because a btree entry is capped near
-- 2700 bytes and a message with a real body is larger than that: indexing the
-- payload directly would refuse to hold long messages, which is precisely the
-- kind nobody notices until somebody writes one. jsonb renders canonically —
-- it sorts keys and drops duplicates when it parses — so two encodings of one
-- message hash the same, which is what makes the comparison mean what it says.
--
-- A COLLISION HERE COSTS A HOLD, NOT A SEND. If two genuinely different
-- messages ever hashed alike, the second refusal would reuse the first row and
-- its review; the rep would see the wrong draft in their review and the second
-- message would not be held. That is visible and recoverable. It cannot cause a
-- send: nothing here fires anything, and the resume path re-runs every gate.
CREATE UNIQUE INDEX scheduled_send_one_held_message_per_seat
    ON scheduled_send (
        held_reason,
        scheduled_by,
        principal_kind,
        payload_version,
        origin_kind,
        coalesce(agent_actor_id, ''),
        coalesce(agent_passport_id, '00000000-0000-0000-0000-000000000000'::uuid),
        coalesce(anchor_activity_id, '00000000-0000-0000-0000-000000000000'::uuid),
        md5(coalesce(origin_links, '[]'::jsonb)::text),
        md5(coalesce(also_links, '[]'::jsonb)::text),
        md5(payload::text))
    WHERE status = 'held' AND held_reason = 'send_refused';
