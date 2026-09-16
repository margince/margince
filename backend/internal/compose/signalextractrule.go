// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/modules/activities"

// THE RULE the signal extractor reads conversations by, spelled once.
//
// Two readers ask about it and they ask different questions. The pass asks
// WHICH threads to read now and acts on the answer; the pipeline trace asks why
// this one conversation was or was not read, and explains the answer to the
// member whose mail it is. A trace that re-spelled the rule would be correct on
// the day it was written and silently wrong the first time an arm moved — which
// is the exact defect class the trace exists to expose, so reproducing it inside
// the trace would be the worst possible bug. The lesson is
// activities.ClassifyBacklogPredicate's, one surface over.
//
// So each arm is named here, and both queries below are COMPOSED from the names.
// They cannot disagree, because there is nothing to disagree with.

// conversationCTE folds the thread's messages into the one row both queries ask
// about. scope narrows the messages it reads — empty for the pass, which wants
// every conversation, and one thread's key for the trace, which wants one and
// must not aggregate the installation to find it.
func conversationCTE(scope string) string {
	return conversationHead + scope + conversationTail
}

// The CTE either side of the scope, as package-level text rather than one
// function body: it is a hundred lines of SQL, and a function holding it is a
// hundred-line function — which is a length rule about how much a reader must
// hold at once, answering a question about a string literal.
//
// A var rather than a const because the head composes the company reach set,
// which activities renders.
var conversationHead = `
		WITH conversation AS (
			SELECT a.thread_key,
			       max(a.occurred_at) AS newest,
			       -- The thread's own lower end, as the pair the cursor is.
			       -- min(id) FILTER on the minimum instant, because the oldest
			       -- MESSAGE is not the smallest id at any other instant.
			       min(a.occurred_at) AS oldest,
			       -- min() has no uuid overload, so the id is compared as text.
			       -- A uuidv7 orders the same either way — its text form is the
			       -- big-endian bytes in hex — and the read below casts it back.
			       min(a.id::text) FILTER (WHERE a.occurred_at = (
			         SELECT min(b.occurred_at) FROM activity b
			          WHERE b.thread_key = a.thread_key AND b.kind = 'email'
			            AND b.archived_at IS NULL AND b.captured_by LIKE 'connector:%'
			       ))::uuid AS oldest_id,
			       count(DISTINCT a.id) AS message_count,
			       min(ro.company_id::text) AS one_company,
			       count(DISTINCT ro.company_id) AS company_count,
			       -- The same question one level down. A thread is read as one
			       -- conversation about one thing, and the model is shown all of
			       -- it — so a thread spanning two projects at one client has no
			       -- single body of work its findings belong to, and whichever
			       -- project the extractor happened to file them against would be
			       -- wrong for half the messages (#2287).
			       --
			       -- Counted from the link directly rather than through a reach
			       -- set: a project is named on the activity or it is not, where
			       -- a company is also reached through the contacts on it.
			       count(DISTINCT pl.project_id) AS project_count,
			       -- Shared only when EVERY message is: the model is shown the
			       -- whole conversation, so what it writes is as private as the
			       -- most private thing it read.
			       --
			       -- A message with no links at all counts as shared, which is
			       -- the link-less note rule auth.ActivityDiscoverClause already
			       -- applies — its empty link set reads as visible. Calling it
			       -- private here would disagree with the gate the reader
			       -- actually faces, and withhold a finding about mail anyone
			       -- may open.
			       --
			       -- A thread whose messages answer to DIFFERENT owners has no
			       -- one reader every message admits; naming one of them would
			       -- hand that contact the others' content through the summary.
			       -- It names nobody, and the WHERE below then refuses it.
			       bool_and(coalesce(vis.shared, true)) AS shared,
			       CASE WHEN count(DISTINCT vis.private_owner) > 1 THEN NULL
			            ELSE min(vis.private_owner)
			       END AS private_owner,
			       -- A message whose AUDIENCE a human or a classifier limited
			       -- takes the whole thread out of the pass, and does NOT fall
			       -- back to its mailbox owner the way capture-private RECORDS
			       -- do. The two look alike and are not: a record's owner
			       -- visibility says one contact is the reader, so a summary
			       -- addressed to that contact discloses nothing new, while a
			       -- limited audience says the message's content is withheld
			       -- from readers who can still see the records it is filed
			       -- against — and an owner-scoped signal is a durable,
			       -- searchable restatement of it that outlives the message's
			       -- own limit. There is no owner for whom extracting it is
			       -- free, so the thread is not offered.
			       --
			       -- bool_and IGNORES nulls, so this is only a whole-thread test
			       -- because activity.audience is NOT NULL with a 'workspace'
			       -- default. A nullable audience would make a limited thread
			       -- read as open the moment one row's value went missing, which
			       -- is why the sibling arms above coalesce and this one does
			       -- not need to.
			       -- Over EVERY email on the conversation, which is why it is a
			       -- correlated subquery rather than an aggregate: this CTE
			       -- keeps only connector-captured mail, and the window read
			       -- that follows is not connector-scoped. A hand-logged
			       -- limited message on a captured thread is therefore one the
			       -- offer would not see and the reading would.
			       --
			       -- Its body is excluded there, so no limited text reaches the
			       -- model either way. What this refuses is the shape: a thread
			       -- summarised while one of its messages is withheld from the
			       -- summary's readers is a partial account presented as a
			       -- whole one, and the reader cannot tell.
			       --
			       -- NOT EXISTS over the limited ones, never bool_and over the
			       -- open ones: the two differ on a thread whose every message
			       -- is somehow absent, and only this direction refuses it.
			       NOT EXISTS (SELECT 1 FROM activity t
			                    WHERE t.thread_key = a.thread_key AND t.kind = 'email'
			                      AND t.archived_at IS NULL
			                      AND t.audience <> 'workspace') AS every_message_open
			  FROM activity a
			  LEFT JOIN (` + activities.CompanyReachSet() + `) ro ON ro.activity_id = a.id
			  -- Safe to widen the row set: every aggregate above is DISTINCT,
			  -- bool_and or min, so a message repeated once per project link
			  -- contributes the same value it did.
			  LEFT JOIN activity_link pl
			    ON pl.activity_id = a.id AND pl.entity_type = 'project'
			  -- Who may read this message, asked of the records it is filed
			  -- against. A message is discoverable when ANY of its links is
			  -- (auth.ActivityDiscoverClause), so one workspace-visible link
			  -- shares it; only an activity whose every link is capture-private
			  -- belongs to one contact, and then that contact is its owner.
			  LEFT JOIN LATERAL (
			    SELECT bool_or(coalesce(vp.visibility, vo.visibility, 'workspace') <> 'owner') AS shared,
			           min(coalesce(vp.owner_id, vo.owner_id)::text)
			             FILTER (WHERE coalesce(vp.visibility, vo.visibility) = 'owner')
			             AS private_owner
			      FROM activity_link vl
			      LEFT JOIN contact vp ON vp.id = vl.contact_id
			      LEFT JOIN company vo ON vo.id = vl.company_id
			     WHERE vl.activity_id = a.id
			  ) vis ON true
			 WHERE a.thread_key IS NOT NULL AND a.kind = 'email'
			   AND a.archived_at IS NULL AND a.captured_by LIKE 'connector:%'`

const conversationTail = `
			 GROUP BY a.thread_key
		)
`

// The arms of the offer, each with the reason it is an arm. A member reading the
// trace sees one of these as the answer, so the wording of the comment and the
// wording of the rung's reason are about the same fact.
const (
	// Exactly one account across the whole thread. A conversation touching two
	// would have its events filed against whichever the join happened to pick,
	// and a signal on the wrong account is worse than no signal.
	threadReachesOneAccount = `c.company_count = 1`

	// At MOST one project, where the company must be exactly one: most mail
	// carries no project at all, and a thread about no particular body of work
	// is still a thread worth reading. Two is the refusal.
	threadIsOneBodyOfWork = `c.project_count <= 1`

	// No message on the thread limited from the summary's readers. A thread
	// summarised while one of its messages is withheld from that summary's
	// audience is a partial account presented as a whole one.
	threadIsFullyOpen = `c.every_message_open`

	// A conversation nobody else may read, whose reader cannot be named, is not
	// offered at all. Reading it would produce a finding with no owner to answer
	// to, and a signal that names no owner is a shared one — so the
	// unattributable case would resolve, silently, to the widest possible
	// audience. Refusing it is the only answer that fails the safe way.
	threadHasANamedReader = `(c.shared OR c.private_owner IS NOT NULL)`

	// Settled: the extractor reads a conversation that has stopped moving, so a
	// thread still in progress is not read YET rather than not read.
	threadHasSettled = `c.newest <= $1`

	// Parked: this exact conversation state has been refused as often as it may
	// be, recently. Two things release it. A message added to the thread changes
	// newest or the count, so the pin stops matching and the text is no longer
	// the text that was refused. And the park itself expires, because what
	// refused the reading was a text AND a model, and the model changes.
	threadIsParked = `(coalesce(s.refusals, 0) >= $3
		            AND s.refused_activity_at IS NOT DISTINCT FROM c.newest
		            AND s.refused_message_count IS NOT DISTINCT FROM c.message_count
		            AND s.scanned_at > $4)`

	// Due when the conversation has MOVED in any way it can. The timestamp
	// misses a message inserted at the same instant and a backfill that adds
	// older ones; the count sees both. And the ACCOUNT moves without the
	// conversation moving at all — two of the three arms are live
	// relationships, so a contact changing employer re-points every quiet
	// thread they are on. Read for one account is not read for another.
	//
	// The last arm is history left BELOW the cursor: a window is six messages
	// and a thread can be longer, so a pass that read one window and recorded
	// the whole count would otherwise never be offered the rest. This is what
	// makes "walks back a window per pass" true rather than described.
	threadHasMoved = `(s.thread_key IS NULL
		        OR s.last_activity_at < c.newest
		        OR s.message_count <> c.message_count
		        OR s.resolved_company_id IS DISTINCT FROM c.one_company::uuid
		        OR (s.scanned_from IS NOT NULL
		            AND (c.oldest, c.oldest_id) < (s.scanned_from, s.scanned_from_id)))`
)

// dueThreadsQuery is the queue itself: the arms above, all of them, newest
// first.
//
// $1 settled instant, $2 per-pass cap, $3 refusal cap, $4 park cutoff.
var dueThreadsQuery = conversationCTE("") + `		SELECT c.thread_key, c.one_company::uuid, c.newest, c.message_count,
		       CASE WHEN c.shared THEN NULL ELSE c.private_owner::uuid END,
		       -- The two ends a previous read reached, so this one can tell new
		       -- mail from a backfill. Both null on a thread never scanned.
		       --
		       -- NULLIF over -infinity, which is what a REFUSAL writes: the row
		       -- exists to hold the refusal count, and no read has happened, so
		       -- "how far did reading get" has the same answer as a thread with
		       -- no row at all. It is also not a time.Time, so scanning it
		       -- would fail the whole pass.
		       NULLIF(s.last_activity_at, '-infinity'), s.scanned_from, s.scanned_from_id
		  FROM conversation c
		  LEFT JOIN signal_thread_scan s ON s.thread_key = c.thread_key
		 WHERE ` + threadReachesOneAccount + `
		   AND ` + threadIsOneBodyOfWork + `
		   AND ` + threadIsFullyOpen + `
		   AND ` + threadHasANamedReader + `
		   AND ` + threadHasSettled + `
		   AND NOT ` + threadIsParked + `
		   AND ` + threadHasMoved + `
		 ORDER BY c.newest DESC
		 LIMIT $2`

// threadOfferQuery answers the SAME arms for ONE conversation, so the trace can
// say which of them the thread failed rather than "not read".
//
// Every arm is reported, not just the first that refuses: a member asking why
// their conversation was not read is owed the whole answer, and a thread can
// fail two arms at once.
//
// $1 settled instant, $3 refusal cap, $4 park cutoff, $5 thread key. $2 is the
// pass cap and is unused here, which is what keeps the bind positions identical
// to the queue's — the arms carry their own $N and a renumbering would mean two
// spellings of each.
var threadOfferQuery = conversationCTE(" AND a.thread_key = $5") + `
		SELECT ` + threadReachesOneAccount + `,
		       ` + threadIsOneBodyOfWork + `,
		       ` + threadIsFullyOpen + `,
		       ` + threadHasANamedReader + `,
		       ` + threadHasSettled + `,
		       ` + threadIsParked + `,
		       ` + threadHasMoved + `,
		       s.thread_key IS NOT NULL
		  FROM conversation c
		  LEFT JOIN signal_thread_scan s ON s.thread_key = c.thread_key
		 LIMIT $2`

// threadOfferPassCap keeps threadOfferQuery's $2 bound. The query reads ONE
// thread key, so it returns one row whatever this says; it exists so the arms
// above can be shared verbatim rather than renumbered per caller.
const threadOfferPassCap = 1
