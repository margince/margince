-- A buyer's thread or comment lands once, however many times it is delivered.
--
-- POST /v1/public/rooms/threads and POST /v1/public/rooms/{id}/comments are
-- buyer-facing and unauthenticated, and neither had any protection against a
-- repeated delivery: a double-click, a mobile retry or a proxy replay posted the
-- comment twice into a room the seller then reads.
--
-- The transport's own idempotency could not cover them. That middleware keys a
-- claim on the acting principal (compose/idempotency.go reads principal.Actor), and
-- a buyer edge has no principal — which is why no /public/ route appears in
-- replayableOperations, and why the one other public write that needed this,
-- bookPublicMeeting, carries its own unique index instead.
--
-- So the key is a column the client mints, scoped by the room it belongs to.
-- Nullable, because a client that sends none gets exactly the behaviour it had
-- before rather than a refusal, and partial so those rows do not collide with each
-- other on NULL.
--
-- NOT content-derived. A key over (room_id, author, body) would refuse a buyer who
-- means to say the same thing twice, which is a thing buyers do.
-- Built plainly, not CONCURRENTLY: this runner holds each migration in one
-- transaction, where CONCURRENTLY cannot run. So each build takes a write lock on
-- its table for the scan. Both tables are per-room conversation rows rather than a
-- high-volume log, and the scan is over a column that is NULL in every existing row,
-- so the window is short — but it is a window, and a room taking a comment at that
-- moment waits for it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_room_thread ADD COLUMN request_id uuid;
CREATE UNIQUE INDEX uq_deal_room_thread_request
    ON deal_room_thread (room_id, request_id) WHERE request_id IS NOT NULL;

ALTER TABLE deal_room_comment ADD COLUMN request_id uuid;
CREATE UNIQUE INDEX uq_deal_room_comment_request
    ON deal_room_comment (room_id, request_id) WHERE request_id IS NOT NULL;
