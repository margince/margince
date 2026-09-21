// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The eligibility query behind "who is waiting", as ONE statement.
//
// Its own file because it is the thing three callers share and must never fork:
// the Worklist's workspace-wide read, the entity-scoped list filter, and the
// hidden-backlog guardrail. Every rule that decides whether a contact is waiting
// lives here — the anti-joins, the machine-sender exclusion, the horizon, the
// sales-link requirement, the live-record predicates — and a caller restating
// any of them would be a second answer to one question, wrong the first time
// either copy was edited.
//
// waiting.go holds what READS it; this holds what it says.

import "fmt"

// waitingRepliesSQL includes outstanding requests and incidental unanswered
// sales conversations. Requests survive replies, age and closed deals until
// explicit resolution; incidental mail retains the conversation guardrails.
// All eligibility predicates precede the cap. Thread comparisons stay within
// the same medium and read instant; unrelated or future mail cannot answer one.
// Unthreaded mail is admitted only with request evidence.
const waitingRepliesSQL = `
	SELECT a.id, a.kind, COALESCE(a.subject, ''),
	       COALESCE((array_agg(sender.address ORDER BY sender.address)
	                 FILTER (WHERE sender.address IS NOT NULL))[1], ''),
	       a.occurred_at,
	       -- One row per message however many records it is filed under. There
	       -- is no max(uuid) in Postgres, so the pick is the first by text
	       -- order: arbitrary but STABLE, which is what a card needs — the same
	       -- message must not point at the contact on one read and the company
	       -- on the next.
	       COALESCE((array_agg(wl.contact_id ORDER BY wl.contact_id::text)
	                 FILTER (WHERE wl.contact_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.company_id ORDER BY wl.company_id::text)
	                 FILTER (WHERE wl.company_id IS NOT NULL))[1],
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
	       -- lead, contact, company.
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
	       -- reports: the row would name deal D1 and bill its wait to the contact
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
	       -- Whether this workspace has ever written on this thread before the
	       -- message arrived. It is REPORTED, never used to exclude: a client
	       -- that strips References gives each message its own thread key, and
	       -- an exclusion would then hard-drop a live customer with nothing on
	       -- screen to say so. The caller demotes what it cannot prove instead,
	       -- so the cost of being wrong is a scroll rather than a lost sale.
	       --
	       -- Ungated on purpose, like the sales-link arm it sits beside:
	       -- eligibility that depended on the reader would make the same message
	       -- engaged for one colleague and cold for another.
	       -- The classifier's answer, or empty when nobody has judged this
	       -- message. Uncertain intent remains reviewable without claiming priority.
	       coalesce(a.owed_verdict, ''), coalesce(a.capture_label, ''),
	       -- Whether every header recipient names somebody OTHER than the
	       -- reader — a thread they can see because it reached their mailbox,
	       -- addressed to a colleague.
	       --
	       -- Read against the reader's OWN addresses, and it has to be, because
	       -- neither participant row answers it alone. Capture stamps the
	       -- mailbox owner with a user_id and no address, which says the mail
	       -- arrived here rather than who it was written to; and mailmap's
	       -- otherParties deliberately DROPS the owner's own address from the
	       -- header list, so their name is never among the address-bearing
	       -- rows even when the sender wrote to them directly.
	       --
	       -- So the question is asked the other way round: is there an
	       -- address-bearing recipient, and is NONE of them this reader? That
	       -- is true exactly when the mail was written to somebody else.
	       --
	       -- An EMPTY address list answers false, like the colleague-domain
	       -- rule and for the same reason: a reader whose own addresses cannot
	       -- be resolved must not have every waiting customer quietly demoted.
	       --
	       -- REPORTED, never used to exclude. The caller demotes what it
	       -- cannot prove.
	       (coalesce(array_length(%[17]s::text[], 1), 0) > 0
	        AND EXISTS (
	          SELECT 1 FROM activity_participant anyTo
	           WHERE anyTo.activity_id = a.id AND anyTo.role = 'to'
	             AND coalesce(anyTo.address, '') <> '')
	        AND NOT EXISTS (
	          SELECT 1 FROM activity_participant addressed
	           WHERE addressed.activity_id = a.id AND addressed.role = 'to'
	             AND lower(addressed.address) = ANY(%[17]s::text[]))),
	       EXISTS (
	         SELECT 1 FROM activity ours
	          WHERE ours.thread_key = a.thread_key
	            AND ours.kind = a.kind
	            AND ours.channel_provider IS NOT DISTINCT FROM a.channel_provider
	            AND ours.direction = 'outbound'
	            AND ours.archived_at IS NULL
	            AND (ours.occurred_at, ours.id) < (a.occurred_at, a.id)),
	       COALESCE(
	         (array_agg(ownerDeal.owner_id ORDER BY ownerDeal.id::text)
	          FILTER (WHERE ownerDeal.owner_id IS NOT NULL))[1],
	         (array_agg(ownerLead.owner_id ORDER BY ownerLead.id::text)
	          FILTER (WHERE ownerLead.owner_id IS NOT NULL))[1],
	         (array_agg(ownerContact.owner_id ORDER BY ownerContact.id::text)
	          FILTER (WHERE ownerContact.owner_id IS NOT NULL))[1],
	         (array_agg(ownerCompany.owner_id ORDER BY ownerCompany.id::text)
	          FILTER (WHERE ownerCompany.owner_id IS NOT NULL))[1],
	         '00000000-0000-0000-0000-000000000000'::uuid),
	       -- Whether this message belongs to a conversation at all.
	       --
	       -- Two of the three things a rep may do with a waiting row are keyed
	       -- on the thread: dismissing it workspace-wide judges the THREAD, and
	       -- snoozing until a reply wakes on a later message with the same
	       -- thread_key. A row without one can do neither, so the caller must
	       -- know before it offers them.
	       a.thread_key IS NOT NULL AND a.thread_key <> ''
	  FROM activity a
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
	  -- Who wrote. The sender participant is where capture records the address,
	  -- and it is the only evidence at this level that tells a contact apart
	  -- from a notification service.
	  LEFT JOIN activity_participant sender
	         ON sender.activity_id = a.id AND sender.role = 'from'
	  LEFT JOIN deal openDeal ON openDeal.id = wl.deal_id
	                         AND %[8]s
	  -- The ownership walk, all four off the gated link join above.
	  LEFT JOIN deal ownerDeal ON ownerDeal.id = wl.deal_id
	  LEFT JOIN lead ownerLead ON ownerLead.id = wl.lead_id
	  LEFT JOIN contact ownerContact ON ownerContact.id = wl.contact_id
	  LEFT JOIN company ownerCompany ON ownerCompany.id = wl.company_id
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
	   -- A message with no thread key is judged by the rules below like any
	   -- other, rather than being required to carry request evidence first.
	   --
	   -- The evidence it was asked for is evidence this queue PRODUCES. The
	   -- owed-verdict pass reads its backlog from this query narrowed to
	   -- unjudged rows, so a row excluded here is never judged, never gains a
	   -- verdict, and is excluded again on the next pass — the exclusion fed
	   -- itself. A client wrote "Dienstag 14 Uhr würde bei uns passen", the
	   -- deal card said "Their move. Nobody here is owed an answer.", and no
	   -- pass could ever reach the message to disagree.
	   --
	   -- Nothing is loosened by admitting it. The reply anti-joins below
	   -- compare thread keys with plain equality and never NULL-match them, so
	   -- a threadless row simply finds no reply and stays waiting; the machine
	   -- and colleague rules sit above the scan cap and still apply.
	   AND NOT EXISTS (SELECT 1 FROM activity request_task
	     WHERE request_task.source_system = '` + EmailRequestTaskSource + `'
	       AND request_task.source_activity_id = a.id
       AND (request_task.is_done OR (request_task.archived_at IS NULL
         AND (request_task.assignee_id = $%[10]d OR $%[10]d = '00000000-0000-0000-0000-000000000000'::uuid))))
	   -- Age bounds incidental unanswered mail, never a recognized request.
	   -- Old requests remain reviewable; the attention rank decides prominence.
	   AND ((` + requestCandidateSQL + `) OR a.occurred_at >= $%[1]d - make_interval(days => %[5]d)
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
	            AND (sales.contact_id IS NOT NULL
	              OR sales.company_id IS NOT NULL
	              OR EXISTS (SELECT 1 FROM deal d
	                          WHERE d.id = sales.deal_id AND ( %[6]s OR (d.archived_at IS NULL AND (` + requestCandidateSQL + `))))
	              OR EXISTS (SELECT 1 FROM lead ld
	                          WHERE ld.id = sales.lead_id AND ( %[7]s OR (ld.archived_at IS NULL AND (` + requestCandidateSQL + `)))))))
	   -- A COLLEAGUE is not a customer waiting.
	   --
	   -- Our own domains are read through the seam that owns them and passed in
	   -- as a list, so the test runs here rather than over the rows that came
	   -- back: after the cap, two hundred internal threads fill the scan and
	   -- push a real customer past it — the same reason every rule above is
	   -- ordered where it is.
	   --
	   -- An EMPTY list admits everyone. A deployment that cannot say which
	   -- domains are its own must not guess: a queue with colleagues in it is a
	   -- visible annoyance, and a queue that dropped a customer whose domain
	   -- merely resembles ours is the silent failure this file exists to avoid.
	   --
	   -- Matched on the address's domain, and on a subdomain of one of ours, the
	   -- way the seam's own set does — mail from a departmental host is still
	   -- from a colleague.
	   AND (%[14]s OR NOT %[15]s)
	   -- The obvious machines, excluded BEFORE the cap. Filtering them after
	   -- LIMIT lets two hundred notification threads fill the scan and push a
	   -- real customer past it, and the page then says nobody is waiting —
	   -- which is the one answer this source must never get wrong.
	   --
	   -- Deliberately coarse: it removes what nothing could mistake for a
	   -- contact, and the caller's own rule (capture's address list, which
	   -- knows the operator's allowlist) still runs over what survives.
	   AND ((` + outstandingRequestSQL + `) OR NOT EXISTS (
	         SELECT 1 FROM activity_participant machine
	          WHERE machine.activity_id = a.id
	            AND machine.role = 'from'
	            AND (machine.address ILIKE '%%noreply%%'
	              OR machine.address ILIKE '%%no-reply%%'
	              OR machine.address ILIKE '%%do-not-reply%%'
	              OR machine.address ILIKE '%%donotreply%%'
	              OR machine.address ILIKE '%%notification%%'
	              OR machine.address ILIKE '%%mailer-daemon%%')))
	   AND %[18]s
	   AND ((` + requestCandidateSQL + `) OR NOT EXISTS (
	         SELECT 1 FROM activity newer
	          WHERE newer.thread_key = a.thread_key
	            AND newer.kind = a.kind
	            AND newer.channel_provider IS NOT DISTINCT FROM a.channel_provider
	            AND newer.direction = 'inbound'
	            AND newer.archived_at IS NULL
	            AND newer.occurred_at <= $%[1]d
	            AND (newer.occurred_at, newer.id) > (a.occurred_at, a.id)))
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
	   -- A snooze lifts on whatever it named: its own moment, the counterparty
	   -- writing back, or a meeting being over. The row comes back when that
	   -- happens rather than waiting for somebody to remember it.
	   --
	   -- not_mine carries no moment and does not lift at all. Ending it when the
	   -- linked record changes hands would be the kinder rule, and it is not
	   -- implemented: a message reaches its owner through a contact, an
	   -- company, a deal or a lead, so the re-arm is a consumer over four
	   -- ownership events rather than a clause here. Until that exists the
	   -- judgement stands until its reader withdraws it, and the contract says
	   -- so rather than promising the re-arm.
	   AND NOT EXISTS (
	         SELECT 1 FROM activity_reader_state mine
	          WHERE mine.activity_id = a.id
	            AND mine.reader_id = $%[10]d
	            AND (mine.state = 'not_mine'
	              OR (mine.state = 'snoozed' AND NOT %[16]s)))
	 GROUP BY a.id, a.kind, a.subject, a.occurred_at
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
	 --
	 -- %[19]s is the keyset continuation, empty on the first page. The machine
	 -- rule this scan can express is a coarse subset of the real one — the full
	 -- test reads a registrable domain against a transactional baseline, which
	 -- is a public-suffix question rather than a LIKE — so the caller filters
	 -- what survives and asks for another page when too much of it went. The
	 -- cap bounds ONE page; the caller bounds how many it will ask for.
	 HAVING TRUE %[19]s
	 ORDER BY a.occurred_at DESC
	 LIMIT %[4]d`

// messageSnoozeLiftedSQL is whether a reader's snooze on one waiting message is
// over, as a boolean expression the waiting query embeds.
//
// The three conditions and the two columns are the same ones the brief's queue
// uses, and the answer is deliberately NOT shared with it: what "they replied"
// means differs. A brief item waits on a DEAL, so any inbound linked to that
// deal answers it. A waiting message waits on a CONVERSATION, so only a newer
// inbound on the same thread does — an unrelated mail from the same customer
// about a different subject is not the reply the rep was waiting for.
//
// asOf is the instant to judge against; it is the caller's own placeholder, and
// every other term here is a fixed column reference.
// The content gate is taken as a fragment the caller renders for the `back`
// alias, exactly as it renders one for `a`. Without it a reply the reader may
// not see would still lift their snooze, and the row reappearing on their day
// is itself the disclosure that it arrived.
func messageSnoozeLiftedSQL(asOf, backContent string) string {
	return fmt.Sprintf(`(CASE mine.reopen_on
		WHEN 'time' THEN mine.snoozed_until <= %[1]s
		-- The same thread identity the waiting query matches replies by: key
		-- alone shares a namespace across kinds and providers, so a crafted
		-- References header could otherwise lift a snooze on a conversation
		-- the sender has nothing to do with.
		WHEN 'reply' THEN EXISTS (
			SELECT 1 FROM activity back
			WHERE back.archived_at IS NULL
			  AND %[3]s
			  AND back.direction = 'inbound'
			  AND back.thread_key = a.thread_key
			  AND back.kind = a.kind
			  AND back.channel_provider IS NOT DISTINCT FROM a.channel_provider
			  AND back.occurred_at > mine.set_at
			  AND back.occurred_at <= %[1]s)
		-- The same rule the brief's queue applies, and for the same reasons:
		-- over means ENDED (start plus duration, not start), a meeting that
		-- will never happen counts as over, and kind = 'meeting' is what makes
		-- reopen_ref mean a meeting rather than any activity id.
		WHEN 'meeting' THEN EXISTS (
			SELECT 1 FROM activity m
			WHERE m.id = mine.reopen_ref AND m.kind = '%[2]s'
			  AND (m.archived_at IS NOT NULL
			       OR m.meeting_status IN ('canceled', 'no_show')
			       OR m.occurred_at
			          + make_interval(secs => coalesce(m.duration_seconds, 0)) <= %[1]s))
		ELSE false
	END)`, asOf, KindMeeting, backContent)
}
