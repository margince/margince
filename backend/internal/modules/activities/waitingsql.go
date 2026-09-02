// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The eligibility query behind "who is waiting", as ONE statement.
//
// Its own file because it is the thing three callers share and must never fork:
// the Worklist's workspace-wide read, the entity-scoped list filter, and the
// hidden-backlog guardrail. Every rule that decides whether a person is waiting
// lives here — the anti-joins, the machine-sender exclusion, the horizon, the
// sales-link requirement, the live-record predicates — and a caller restating
// any of them would be a second answer to one question, wrong the first time
// either copy was edited.
//
// waiting.go holds what READS it; this holds what it says.

// waitingRepliesSQL finds, per thread, the newest inbound with no later
// outbound in the same thread — where the thread is a SALES conversation the
// workspace is answerable for, recent enough to still be one.
//
// Every eligibility rule is applied before ORDER BY and LIMIT, so the cap falls
// on qualified rows only. A rule applied after the cap reads as a working
// filter and fails as a silent one: the scan fills with rows the rule would
// have removed, the customer behind them never arrives, and the page reports an
// empty queue with nothing to say it was truncated.
//
// NOT EXISTS rather than a window function or a join: it expresses the question
// directly — "nobody wrote back after this" — and it stops at the first later
// outbound rather than materializing every thread's history to sort it.
//
// The outbound side deliberately ignores the audience arm. A reply this reader
// may not READ still answered the customer, and skipping it would report a
// message as unanswered because the answer was somebody else's to see — the
// worst failure available here, since it sends a rep to write a second reply.
//
// A thread is matched within ONE medium: same kind, same channel provider. A
// mail thread key comes from headers the sender controls, and channel keys
// share the flat namespace with them, so comparing keys alone lets a crafted
// References value silence an unrelated conversation. The capture side's own
// reply detector matches the same way.
//
// The anti-joins are bounded by the read instant too, so the answer is a
// snapshot: a message dated in the future — mail carries the sender's own Date
// header — cannot suppress a thread that is genuinely waiting now.
//
// Equal timestamps are broken by id, because second-precision mail makes ties
// ordinary and both halves of "newest inbound, no later outbound" would
// otherwise be wrong at once.
//
// A message with NO thread_key is excluded rather than matched loosely. SQL
// equality would never join two NULLs, and IS NOT DISTINCT FROM joins them ALL
// — so an unthreaded message would be silenced by any other unthreaded
// outbound in the workspace, and one unthreaded reply would hide every
// unthreaded question at once. Excluding them under-reports, which is the
// direction that costs a row rather than a customer.
const waitingRepliesSQL = `
	SELECT a.id, COALESCE(a.subject, ''),
	       COALESCE((array_agg(sender.address ORDER BY sender.address)
	                 FILTER (WHERE sender.address IS NOT NULL))[1], ''),
	       a.occurred_at,
	       -- One row per message however many records it is filed under. There
	       -- is no max(uuid) in Postgres, so the pick is the first by text
	       -- order: arbitrary but STABLE, which is what a card needs — the same
	       -- message must not point at the person on one read and the company
	       -- on the next.
	       COALESCE((array_agg(wl.person_id ORDER BY wl.person_id::text)
	                 FILTER (WHERE wl.person_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.organization_id ORDER BY wl.organization_id::text)
	                 FILTER (WHERE wl.organization_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.deal_id ORDER BY wl.deal_id::text)
	                 FILTER (WHERE wl.deal_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       -- Whether an open deal this reader can SEE is on this thread, which
	       -- is what lets a long wait with money on it stay in the day.
	       --
	       -- Off the visibility-gated join, like every record id above it. Read
	       -- off an ungated one it would answer "a deal exists" rather than "you
	       -- can see a deal", and a reader would learn the first by watching a
	       -- row they can see decline to go stale.
	       bool_or(openDeal.id IS NOT NULL),
	       -- WHO owes the reply, first owner found down the precedence: deal,
	       -- lead, person, organization.
	       --
	       -- COALESCE over four aggregates rather than four correlated
	       -- subqueries: the links are already joined and grouped here, so this
	       -- costs the group it is already paying for.
	       --
	       -- ORDERED BY THE RECORD's id, not by the owner's, and that is the
	       -- whole correctness of it. A message may be filed under two deals —
	       -- uq_activity_link is keyed on (activity, type, id), so a second deal
	       -- link is a legal row — and ordering by owner_id picks the smallest
	       -- OWNER across both. That owner need not own the deal this same query
	       -- reports: the row would name deal D1 and bill its wait to the person
	       -- who owns D2. Ordering by the record id makes each arm walk its links
	       -- in the SAME order the record ids above are picked in.
	       --
	       -- The unowned links are skipped rather than ending the walk, so a
	       -- message on an unowned deal and an owned one is answered by the owner
	       -- who exists. That is deliberately not the same pick as DealID, which
	       -- names the first deal owned or not: the two answer different
	       -- questions — what is this about, and who owes the reply — and the
	       -- struct's own comment says they may differ. What they may NOT do is
	       -- name an owner of some third record, which is what ordering by owner
	       -- id allowed.
	       --
	       -- Every arm reads through wl, the VISIBILITY-GATED link join. An
	       -- owner off an ungated join would name who owns a record this reader
	       -- may not open.
	       COALESCE(
	         (array_agg(ownerDeal.owner_id ORDER BY ownerDeal.id::text)
	          FILTER (WHERE ownerDeal.owner_id IS NOT NULL))[1],
	         (array_agg(ownerLead.owner_id ORDER BY ownerLead.id::text)
	          FILTER (WHERE ownerLead.owner_id IS NOT NULL))[1],
	         (array_agg(ownerPerson.owner_id ORDER BY ownerPerson.id::text)
	          FILTER (WHERE ownerPerson.owner_id IS NOT NULL))[1],
	         (array_agg(ownerOrg.owner_id ORDER BY ownerOrg.id::text)
	          FILTER (WHERE ownerOrg.owner_id IS NOT NULL))[1],
	         '00000000-0000-0000-0000-000000000000'::uuid)
	  FROM activity a
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
	  -- Who wrote. The sender participant is where capture records the address,
	  -- and it is the only evidence at this level that tells a person apart
	  -- from a notification service.
	  LEFT JOIN activity_participant sender
	         ON sender.activity_id = a.id AND sender.role = 'from'
	  LEFT JOIN deal openDeal ON openDeal.id = wl.deal_id
	                         AND %[8]s
	  -- The ownership walk, all four off the gated link join above.
	  LEFT JOIN deal ownerDeal ON ownerDeal.id = wl.deal_id
	  LEFT JOIN lead ownerLead ON ownerLead.id = wl.lead_id
	  LEFT JOIN person ownerPerson ON ownerPerson.id = wl.person_id
	  LEFT JOIN organization ownerOrg ON ownerOrg.id = wl.organization_id
	 WHERE a.kind IN ('email', 'message')
	   AND a.direction = 'inbound'
	   AND a.archived_at IS NULL
	   AND a.occurred_at <= $%[1]d
	   AND %[2]s
	   -- Entity narrowing goes HERE, before WaitingScanCap's LIMIT below: a
	   -- record's own wait can sit outside the oldest WaitingScanCap threads
	   -- workspace-wide, and narrowing after the cap would report nothing
	   -- waiting on the very record this asks about. "TRUE" for the
	   -- workspace-wide Worklist read.
	   AND (%[11]s)
	   AND a.thread_key IS NOT NULL
	   -- Old enough and it is history, not work — UNLESS an open deal is on it.
	   --
	   -- The horizon and the caller's staleness rule have to agree about money,
	   -- or the looser of the two is decoration. The caller keeps a long wait
	   -- that still has a deal behind it, on the ground that there the silence
	   -- IS the problem; a horizon that removed those rows first would make that
	   -- branch unreachable and the rep would never see the one case where a
	   -- half-year of quiet costs something.
	   --
	   -- Before the cap, like every other exclusion here.
	   AND (a.occurred_at >= $%[1]d - make_interval(days => %[5]d)
	     OR EXISTS (
	          SELECT 1 FROM activity_link funded
	          JOIN deal fd ON fd.id = funded.deal_id AND %[9]s
	           WHERE funded.activity_id = a.id))
	   -- A SALES link, or it is not this queue's business.
	   --
	   -- The rule that was missing: this read used to answer "somebody wrote and
	   -- nobody replied", which is true of a rep's dentist. Unanswered is a fact
	   -- about a mailbox; waiting is a fact about a customer, and only a link to
	   -- a record the workspace sells to tells the two apart.
	   --
	   -- Its own EXISTS rather than a predicate on the wl join above, because
	   -- that join is filtered by what the reader may SEE. Qualifying through it
	   -- would make eligibility depend on the reader, so the same message would
	   -- be work for one colleague and personal mail for another.
	   AND (%[13]s OR EXISTS (
	         SELECT 1 FROM activity_link sales
	          WHERE sales.activity_id = a.id
	            AND (sales.person_id IS NOT NULL
	              OR sales.organization_id IS NOT NULL
	              OR EXISTS (SELECT 1 FROM deal d
	                          WHERE d.id = sales.deal_id AND %[6]s)
	              OR EXISTS (SELECT 1 FROM lead ld
	                          WHERE ld.id = sales.lead_id AND %[7]s))))
	   -- The obvious machines, excluded BEFORE the cap. Filtering them after
	   -- LIMIT lets two hundred notification threads fill the scan and push a
	   -- real customer past it, and the page then says nobody is waiting —
	   -- which is the one answer this source must never get wrong.
	   --
	   -- Deliberately coarse: it removes what nothing could mistake for a
	   -- person, and the caller's own rule (capture's address list, which
	   -- knows the operator's allowlist) still runs over what survives.
	   AND NOT EXISTS (
	         SELECT 1 FROM activity_participant machine
	          WHERE machine.activity_id = a.id
	            AND machine.role = 'from'
	            AND (machine.address ILIKE '%%noreply%%'
	              OR machine.address ILIKE '%%no-reply%%'
	              OR machine.address ILIKE '%%do-not-reply%%'
	              OR machine.address ILIKE '%%donotreply%%'
	              OR machine.address ILIKE '%%notification%%'
	              OR machine.address ILIKE '%%mailer-daemon%%'))
	   AND NOT EXISTS (
	         SELECT 1 FROM activity later
	          WHERE later.thread_key = a.thread_key
	            AND later.kind = a.kind
	            AND later.channel_provider IS NOT DISTINCT FROM a.channel_provider
	            AND later.direction = 'outbound'
	            AND later.archived_at IS NULL
	            AND later.occurred_at <= $%[1]d
	            AND (later.occurred_at, later.id) > (a.occurred_at, a.id))
	   AND NOT EXISTS (
	         SELECT 1 FROM activity newer
	          WHERE newer.thread_key = a.thread_key
	            AND newer.kind = a.kind
	            AND newer.channel_provider IS NOT DISTINCT FROM a.channel_provider
	            AND newer.direction = 'inbound'
	            AND newer.archived_at IS NULL
	            AND newer.occurred_at <= $%[1]d
	            AND (newer.occurred_at, newer.id) > (a.occurred_at, a.id))
	   -- Judged NOT a sales conversation, by anybody. A property of the THREAD,
	   -- so it holds for every reader AND for every later reply: one rep
	   -- recognizing the procurement newsletter settles what the conversation
	   -- is, and the next issue of it must not arrive as fresh work.
	   --
	   -- Matched on the same triple the reply anti-joins below use. Keying the
	   -- judgement on one activity id instead let the next inbound revive the
	   -- thread, because that message is a different row.
	   --
	   -- Before the cap, like every rule above it.
	   AND (%[12]s OR NOT EXISTS (
	         SELECT 1 FROM activity_sales_state judged
	          WHERE judged.thread_key = a.thread_key
	            AND judged.kind = a.kind
	            AND judged.channel_provider = coalesce(a.channel_provider, '')))
	   -- Set aside by THIS reader, and only this reader.
	   --
	   -- Judged against the row's CURRENT state rather than against what it was
	   -- at asOf: there is no set_at comparison here, so a judgement made after
	   -- the instant this page was read at still hides its row. In production
	   -- asOf is now() at the top of the same assembly, so the window is
	   -- milliseconds wide and hiding a message somebody just set aside is the
	   -- answer a reader wants. A caller replaying a HISTORICAL instant would
	   -- get today's judgements over that day's messages, and nothing does.
	   --
	   -- A snooze lifts on its own moment, so the row comes back when it is due
	   -- rather than waiting for somebody to remember it.
	   --
	   -- not_mine carries no moment and does not lift at all. Ending it when the
	   -- linked record changes hands would be the kinder rule, and it is not
	   -- implemented: a message reaches its owner through a person, an
	   -- organization, a deal or a lead, so the re-arm is a consumer over four
	   -- ownership events rather than a clause here. Until that exists the
	   -- judgement stands until its reader withdraws it, and the contract says
	   -- so rather than promising the re-arm.
	   AND NOT EXISTS (
	         SELECT 1 FROM activity_reader_state mine
	          WHERE mine.activity_id = a.id
	            AND mine.reader_id = $%[10]d
	            AND (mine.state = 'not_mine'
	              OR (mine.state = 'snoozed' AND mine.snoozed_until > $%[1]d)))
	 GROUP BY a.id, a.subject, a.occurred_at
	 -- NEWEST first, which is the opposite of how the rows are then shown.
	 --
	 -- The cap has to spend its budget on the rows most likely to matter, and
	 -- those are the recent ones: a wait inside a day is urgent, and one past a
	 -- fortnight without an open deal is demoted by the caller the moment it
	 -- arrives. Taking the OLDEST two hundred spent the entire scan on rows
	 -- headed for the bottom of the page and cut the urgent ones before anybody
	 -- saw them — the queue would report nobody waiting on the day it was most
	 -- wrong.
	 --
	 -- The caller sorts oldest-first for display, so what a reader sees is
	 -- unchanged. This decides only WHICH waits survive the bound.
	 ORDER BY a.occurred_at DESC
	 LIMIT %[4]d`
