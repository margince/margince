-- `source` names WHERE a row came from, and a person using this product is one
-- origin with one spelling.
--
-- The Deal Room's tables and the outcome review's frozen rows carry `source`
-- on the same terms as every table the first sweep reached: `ui` and `mcp`
-- name no distinction any reader still makes. The census gate holds the
-- source tree in Go syntax going forward but cannot reach a row already
-- written, which is what this migration is for.
--
-- Safe in this order, unlike the first sweep: that one had to follow a ranking
-- change because retrieval scored rows by `source`. Retrieval reads
-- `captured_by` now, and nothing in the tree branches on a `source` value.
--
-- This collapses the rows that exist NOW. `source` stays free-form on the
-- wire by contract — provenance.Refuse guards only the reserved import
-- namespace, not `ui` — so a third-party client can still post the retired
-- word tomorrow, and this migration is a one-shot sweep, not a standing
-- guarantee.
--
-- The down migration cannot restore the distinction: once two words are one
-- word, nothing in the row says which it was. `captured_by` still records who
-- wrote it, and the passport still records which client they used.

-- COST, stated rather than discovered. None of these tables indexes `source`,
-- so each UPDATE is a full scan, and all seven run in one transaction — the row
-- locks are held until the whole thing commits. lock_timeout bounds how long a
-- statement WAITS for a lock, not how long these scans run. Acceptable because
-- the predicate matches a minority of rows and only rows written since the
-- previous sweep can match at all. One transaction on purpose: a half-applied
-- spelling is worse than a slow one, since nothing afterwards could tell which
-- tables were swept.
SET LOCAL lock_timeout = '3s';

UPDATE activity                 SET source = 'manual' WHERE source IN ('mcp', 'ui');
UPDATE activity_review_response SET source = 'manual' WHERE source IN ('mcp', 'ui');

-- The Deal Room's five written tables. The invitation and session rows carry the
-- server's own 'system' and are deliberately absent.
UPDATE deal_room             SET source = 'manual' WHERE source IN ('mcp', 'ui');
UPDATE deal_room_participant SET source = 'manual' WHERE source IN ('mcp', 'ui');
UPDATE deal_room_document    SET source = 'manual' WHERE source IN ('mcp', 'ui');
UPDATE deal_room_thread      SET source = 'manual' WHERE source IN ('mcp', 'ui');
UPDATE deal_room_comment     SET source = 'manual' WHERE source IN ('mcp', 'ui');

-- The cache holds request bodies a click replays VERBATIM, `source` included,
-- so a card written before this sweep would re-write the retired word into a
-- fresh row the moment somebody clicks it. Clearing it is safe: an absent row
-- is the same cache miss as an unreadable one, and the deal page already
-- rewrites the card on its next read.
DELETE FROM deal_status_card;
