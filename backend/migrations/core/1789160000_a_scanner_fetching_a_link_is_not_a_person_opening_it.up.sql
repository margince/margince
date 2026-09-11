-- A machine fetching a link is not a person opening it.
--
-- confirm_token.opened_at is stamped by the RESOLVER, which every GET of the
-- confirm page runs. The column's own comment calls it "when the page was first
-- opened with this token, so the ask-to-click chain is readable from the row" —
-- and that chain is evidence. A grant's demonstrability rests partly on it: the
-- mail went out at issued_at, the person opened it at opened_at, the answer
-- landed at consumed_at.
--
-- Nothing in that sequence distinguishes a person from a machine. Mail security
-- products fetch every link in a message before delivering it; so do link
-- expanders, preview generators and corporate proxies. Each of those stamps
-- opened_at, and the row then says a named data subject opened their link at a
-- moment they were very probably asleep.
--
-- THE HARM IS THAT IT IS EVIDENCE. An invented opening is not a cosmetic
-- telemetry error: it is a false line in the record a controller would produce
-- to show a consent was freely given. Worse, it is systematically false in the
-- direction that flatters the controller, which is the direction an auditor
-- looks at hardest.
--
-- So the fetch and the opening become two facts.
--
--   first_fetched_at  — when anything first retrieved this link, machine or
--                       person. Operationally useful (a link that is fetched
--                       and never answered says something) and claims nothing
--                       about who.
--   fetch_count       — how many times. A link fetched eleven times in two
--                       seconds was not read eleven times by a human.
--
-- opened_at STAYS, with its meaning narrowed to what it always claimed: a
-- person opened this. Nothing in this migration writes it; the writer that does
-- is the one that can tell the difference, and it lands with this change.
--
-- EXISTING VALUES MOVE RATHER THAN BEING COPIED, and that is the whole
-- question this migration had to answer.
--
-- An opened_at already on a row was written under the old rule, so it may be a
-- scanner's. Copying it into first_fetched_at is right whichever it was:
-- something fetched the link at that moment. LEAVING it in opened_at as well
-- is not, because opened_at now means "a person opened this" — and the subject
-- access export reads exactly that column to tell a subject their link was
-- "opened and not answered" (privacy/sarconsentlinks.go). That is a statement
-- about their own conduct, in their own data export, and for a pre-upgrade row
-- we cannot stand behind it.
--
-- So the value goes where it is true and is cleared where it is not. What is
-- lost is a timestamp we could not attribute; what is kept is every one of
-- those moments, under a column that claims only what we know.

-- THE LOCK IS HELD THROUGH THE BACKFILL, and lock_timeout does not change
-- that: it bounds the WAIT and never the HOLD, so every reader and writer of
-- confirm_token blocks for as long as the UPDATE below takes
-- (docs/how-to/apply-migrations.md §5).
--
-- ACCEPTED HERE ON THE SIZE OF THIS TABLE RATHER THAN WAVED AWAY. A confirm
-- token is minted when somebody asks one contact to confirm their details, so
-- the row count tracks deliberate operator actions rather than traffic, and
-- the UPDATE touches only the rows that were ever fetched
-- (WHERE opened_at IS NOT NULL). What blocks is the confirm-link path alone —
-- minting, resolving and submitting — for the length of one indexed update.
--
-- The documented alternative is to land the column, backfill from application
-- code, then validate. That is the right shape for a table with millions of
-- rows and the wrong one here: it would leave the ambiguous opened_at values
-- standing in the subject access export for however long the backfill took to
-- be written and run, which is the defect this migration exists to end.
SET LOCAL lock_timeout = '3s';

ALTER TABLE confirm_token
    ADD COLUMN first_fetched_at timestamptz,
    ADD COLUMN fetch_count integer NOT NULL DEFAULT 0;

-- Every existing opened_at was a fetch, whatever else it may have been — and
-- only a fetch is what we can say.
UPDATE confirm_token
   SET first_fetched_at = opened_at,
       fetch_count = 1,
       opened_at = NULL
 WHERE opened_at IS NOT NULL;

COMMENT ON COLUMN confirm_token.first_fetched_at IS
    'When anything first retrieved this link — a person, a scanner, a proxy. Claims nothing about who.';
COMMENT ON COLUMN confirm_token.fetch_count IS
    'How many times this link has been retrieved. A link fetched many times in seconds was not read that many times by a human.';
COMMENT ON COLUMN confirm_token.opened_at IS
    'When a PERSON first opened this page, which is evidence and so is written only where a machine fetch can be told apart from a human one. A scanner prefetching the link leaves first_fetched_at and not this.';

