// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The Worklist: the same day the lane feed reads, projected as ONE ranked queue.
//
// It reads through Assemble rather than beside it. Two readers of one day would
// be two answers to "what is waiting on me", and they would drift the first time
// a lane changed — so this is a PROJECTION of the assembled day, and a lane
// added there reaches the queue by being classified here rather than by being
// read again.
//
// What it adds is the part a lane feed cannot: a level, a reason, and a
// consequence. Those are what let a reader compare a duplicate merge with an
// unanswered buyer without reading fourteen panels first.

import (
	"context"
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// worklistPage is how many ranked items one read carries by default.
const worklistPage = 25

// waitingLead is how many unanswered customers lead the page.
//
// Not a cap on the source: the rest are still ranked and still reachable. It is
// a cap on how much of ONE kind a reader meets before they see the others, and
// the number is the answer to "how many can somebody act on this morning"
// rather than to "how many are there".
const waitingLead = 8

// leadResponseBound is how many leads still owed a reply one read carries.
//
// Declared here and passed through the interface, the way plannedCap is, so
// the number the reach figure reports is the number the read actually asked
// for. A source read to its bound reports "more may exist" rather than a total
// it does not know.
const leadResponseBound = 50

// worklistMaxPage is the ceiling the contract publishes. A larger ask is
// clamped rather than refused: the number is a request for how much to draw,
// and answering the most that can be drawn is more useful than an error.
const worklistMaxPage = 100

// Worklist answers the ranked day.
//
// The filter narrows what is CARRIED, never what is read: a source is read,
// classified and then dropped, so the summary's figures describe the same day
// whichever filter is applied.
func (s *Service) Worklist(
	ctx context.Context, scope, filter string, owner ids.UUID, limit int, token string,
) (crmcontracts.Worklist, error) {
	// Resolved BEFORE the day is read: a reader asking for a scope they do not
	// hold gets a refusal rather than a page assembled and then narrowed, and
	// the read they were never entitled to make is not made.
	resolved, err := resolveScope(ctx, scope)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	// Same rule, same moment: whose queue this is, refused rather than narrowed.
	namedOwner, err := s.resolveOwner(ctx, owner)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	// Checked against the RESOLVED scope and owner, not the words the request
	// used. `scope` omitted and `scope=mine` are the same question, and a
	// fingerprint over the raw request would refuse a caller who spelled the
	// default out on page two of a walk they started without it.
	//
	// Refused BEFORE the day is read: a token minted for another question is the
	// caller's mistake, and assembling a page to then discard it spends a dozen
	// lane reads on an answer nobody receives.
	cursor, err := decodeCursor(token, resolved, filter, namedOwner)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	// The producers that CAN be narrowed are narrowed in their own queries,
	// not filtered afterwards: each store bounds what it returns, so a page
	// full of colleagues' rows would hide the reader's own work behind a cut
	// that had already happened.
	// Deeper than the lane feed reads: a batch row counts a pile, and a count
	// taken from a page of ten would report ten over a hundred and fifty.
	reader := s.countingDecisions()
	switch {
	// A named owner outranks the scope word: "their queue" is a narrower
	// question than any of mine/team/all, and answering the wider one would
	// hand back a page that looks like the rep's day and is not.
	case !namedOwner.IsZero():
		reader = reader.forOwner(namedOwner)
	case mineOnly(resolved):
		reader = reader.forReader()
	case resolved == scopeUnassigned:
		reader = reader.forUnowned()
	}
	day, err := reader.Assemble(ctx)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	// The overnight brief ranks deal ids and keeps its figures behind its own
	// endpoint, so its rows arrive naming a deal and saying nothing about it.
	// Filled here, before the projection, so the money is on the row when the
	// ordering reads it — enriching afterwards would rank the deal as unpriced
	// and then print its price, which is the page disagreeing with itself.
	if err := reader.nameTheMoney(ctx, &day); err != nil {
		return crmcontracts.Worklist{}, err
	}
	// The amounts the ordering compares, put into one currency BEFORE anything
	// reads them; basemoney.go states the whole rule.
	if err := reader.priceDayOnto(ctx, day); err != nil {
		return crmcontracts.Worklist{}, err
	}
	// Read beside the assembled day rather than inside it: /attention has its
	// own fourteen-lane promise and this source is not one of its lanes. A
	// refused read is named, never folded into an empty answer.
	waiting, waitingErr := reader.waitingCustomers(ctx, day.AsOf)
	// The same rule for the leads still owed a reply: read beside the day,
	// under the ownership dial this read already resolved, so `mine` narrows
	// in the store's own query rather than by dropping rows afterwards.
	leads, leadsErr := reader.owedLeads(ctx)
	// The NARROWED service, not the shared one. The narrowing happens twice on
	// this path — once when the lanes are read, once when the assembled rows are
	// kept — and both halves read the same taskOwner. Projecting through `s`
	// left the second half seeing no named owner: it fell through to "mine" and
	// returned the READER's own day under the named person's heading, which is
	// the one way this page can be wrong that its reader cannot detect.
	//
	// The two refusals travel INTO the projection rather than onto the finished
	// page. Appended afterwards they reached the reader's warning list but not
	// the readings, so a rep refused the who-is-waiting lane was shown "0
	// customers waiting on an answer" as an exact figure — above the warning
	// that contradicted it.
	// What this reader put at the top, read BEFORE the projection because the
	// projection returns no error: it is the pure part, and a failure here has
	// to reach the caller rather than be swallowed into a page that quietly
	// ignores the override the reader asked for.
	//
	// Carried on a COPY of the service, the way the narrowed reader above is:
	// one Service serves every request, so pins set on it would follow one
	// reader's page onto another's.
	withPins, err := reader.readingPins(ctx)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	out := withPins.worklistFrom(
		ctx, day, resolved, filter, limit, waiting, leads, cursor,
		[]*crmcontracts.WorklistSourceUnavailable{waitingErr, leadsErr})
	out.Scope = crmcontracts.WorklistScope(resolved)
	out.ScopeOptions = scopeOptions(scopeOptionsFor(ctx))
	// The step each deal row suggests, read for the CUT page rather than the
	// whole ranking: a move is drawn and never ranked, so reading one for a row
	// this caller will not receive spends a query on nothing. dealmoves.go
	// states why this reads a cache and never assembles.
	if err := reader.nameTheStep(ctx, out.Queue); err != nil {
		return crmcontracts.Worklist{}, err
	}
	// And the name beside each owner id, over the same cut page and for the same
	// reason: a label is drawn and never ranked, so resolving one for a row this
	// caller will not receive spends a read on nothing.
	if err := reader.nameTheOwners(ctx, out.Queue); err != nil {
		return crmcontracts.Worklist{}, err
	}
	return out, nil
}

// worklistFrom projects an already-assembled day, so a test can drive the
// ranking, the paging and the summary without standing up every lane's reader.
func (s *Service) worklistFrom(
	ctx context.Context, day crmcontracts.Attention, scope, filter string, limit int,
	waiting waitingRead, leads leadRead, cursor worklistCursor,
	// The refusals from the two sources read BESIDE the assembled day. They
	// arrive here rather than being appended to the finished page because the
	// readings have to see them: a refused waiting or leads lane is exactly the
	// case where a tally would otherwise print a confident zero.
	besideTheDay []*crmcontracts.WorklistSourceUnavailable,
) crmcontracts.Worklist {
	if limit <= 0 {
		limit = worklistPage
	}
	if limit > worklistMaxPage {
		limit = worklistMaxPage
	}
	rows := classifyDay(day, day.AsOf, s.money)
	// Longest wait first, so the few that LEAD are the ones most likely to have
	// been forgotten rather than whichever the database returned first.
	waits := make([]ranked, 0, len(waiting.rows))
	for _, customer := range waiting.rows {
		waits = append(waits, classifyWaiting(customer, day.AsOf))
	}
	sort.SliceStable(waits, func(i, j int) bool {
		return waits[i].occurredAt.Before(waits[j].occurredAt)
	})
	// Narrowed BEFORE the crowding mark, through the same filters every other
	// source is narrowed by. Crowding is a fact about position — the ninth wait
	// on THIS page — so marking it over rows the scope then removes would demote
	// a reader's second waiting customer for standing behind seven of a
	// colleague's.
	//
	// One spelling for one rule. This loop used to judge ownership itself, in
	// terms of the WaitingCustomer rather than the ranked row, which is how the
	// two ended up disagreeing: a named owner's queue dropped every wait because
	// the filter below could not see an owner this loop could.
	waits = s.narrowToScope(ctx, waits, scope, s.taskOwner)
	for i, row := range waits {
		// Past the lead, a wait sorts BELOW the other kinds without ceasing to
		// be one: its level still says a customer is waiting, because that is
		// what it is and the summary counts on it. What changes is only where
		// it sits, through the ordering's own last tiebreak.
		//
		// Rewriting the level instead would have told the reader the ninth
		// waiting customer was agreed work, while the row went on saying a
		// buyer wrote last — the page contradicting itself.
		if i >= waitingLead {
			row.crowded = true
		}
		rows = append(rows, row)
	}
	// The leads still owed a first reply, ranked among everything else rather
	// than in a queue of their own. Longest overdue first, so the few that LEAD
	// are the ones most likely to have been forgotten.
	sort.SliceStable(leads.rows, func(i, j int) bool {
		return leads.rows[i].DeadlineAt.Before(leads.rows[j].DeadlineAt)
	})
	for i, lead := range leads.rows {
		row := classifyLead(lead, day.AsOf)
		// Past the lead, an overdue lead sorts BELOW the other kinds without
		// ceasing to be one — the same treatment a ninth waiting customer gets,
		// and for the same reason: its level still states the fact.
		if i >= leadLead {
			row.crowded = true
		}
		rows = append(rows, row)
	}
	// One unanswered message is one row: the deal it belongs to does not also
	// appear as drifting.
	rows = dropDealsAlreadyWaiting(rows)
	// One late reply is one row, not three: the escalation's own task about a
	// lead this queue already shows says nothing the lead row does not.
	rows = dropEscalationTasksAlreadyOwed(rows)
	// One PERSON is one row. "Nobody has spoken to them in sixty days" and
	// "they wrote last week and are waiting" are both true of the same contact
	// and read as a contradiction side by side.
	//
	// Order does not matter among these three, and it is worth saying why
	// rather than leaving the next reader to work it out: each drops rows of a
	// source none of the others READS. This one decides on the waiting rows,
	// and the two above remove only `deal_at_risk` and `task` rows — so the set
	// it judges against is the same wherever it sits. A fourth pass that
	// removed waiting rows would break that, and would have to run last.
	rows = dropDecayAlreadyWaiting(rows)
	// The reader's own override, raised AFTER the dedupe passes and before the
	// ranking. After, because a pin on a row those passes remove is a pin on a
	// row that is not on the page — the day decides what it holds, and the pin
	// only says which of what it holds leads. Before the ranking, because a pin
	// is a level: the ordering, the band heading and the "why here" line all
	// read it, so moving rows after the sort would leave those three saying
	// something the page contradicts.
	rows = applyPins(rows, s.pinned)

	// Whose queue this is, applied to the rows the same way the lane applied it
	// to the query. A row belonging to somebody else is not part of this
	// person's day, and leaving it in would make a manager's answer to "show me
	// Lena's queue" quietly include rows that are not hers.
	//
	// The waiting rows above already ran through this — they had to, before
	// their crowding was decided — and running them through it again keeps
	// nothing new: each filter is a per-row test, so a row it kept once it keeps
	// again.
	rows = s.narrowToScope(ctx, rows, scope, s.taskOwner)
	// The lead read's own bound, which boundedSources cannot see: it reads
	// beside the assembled day rather than as one of its lanes, so the figure
	// travels with the rows.
	bounded := boundedSources(day)
	// Recorded ONLY when the lane ran. reachOf emits a row for every key in
	// this map, so writing false for a source that never read would publish a
	// zero-valued reach entry — the page reporting a source as successfully
	// read and empty, which is exactly the claim an untracked policy must not
	// make.
	if leads.read {
		bounded[sourceLeadResponse] = leads.bounded()
	}
	// The same rule for the who-is-waiting source, and for the same reason it
	// needed one: it is read beside the assembled day rather than as one of its
	// lanes, so boundedSources never saw it and the page reported it complete
	// however deep the scan had gone.
	if waiting.read {
		bounded[sourceWaiting] = waiting.cut
	}
	// Held before the category narrowing, so a filtered-out source still
	// reports what it had. Counting after it erased those sources from reach
	// entirely — a rep narrowing to meetings would read "no tasks" rather than
	// "tasks, not shown", and a filtered-out source that hit its bound took its
	// more_available signal out with it.
	considered := rows
	narrowed := filter != "" && filter != string(crmcontracts.WorklistFilterAll)
	if narrowed {
		rows = keepCategory(rows, crmcontracts.WorklistItemCategory(filter))
	}
	if !narrowed {
		rows = s.foldAndRepin(rows, len(day.NeedsYou) >= batchScanDepth)
	}
	// Cut to the page BEFORE explaining and counting. Ranking the whole set and
	// then slicing left the last returned row comparing itself against a row the
	// caller never received, and the summary describing a queue longer than the
	// one on screen.
	//
	// The resume happens inside the same cut, between the sort and the slice: the
	// candidate order does not depend on `limit` — crowding is marked above, over
	// the whole narrowed set — so page two weighs exactly what page one weighed
	// and continues it rather than re-deciding it.
	// Ranked ONCE, here, whichever way this page is served. A frozen walk takes
	// its sequence from the snapshot and a fresh read takes it from this sort,
	// but both need the comparator to have run: the snapshot's order IS a
	// previous run of it, and the identities it stores were minted from one.
	sortByRank(rows)
	shown, more, reached, walk := s.pageOf(ctx, rows, limit, cursor, scope, filter)
	// RENDERED IN THE ORDER THE PAGER CHOSE, never re-sorted. A fresh page was
	// cut from the ranking above; a frozen walk's sequence is a previous run of
	// that same comparator, and running it again here would return the reader's
	// own rows in today's order rather than the one they were shown.
	ordered := renderInOrder(stampAsOf(shown, day.AsOf), readerOf(ctx))
	bands := bandsOf(ordered)
	// What never answered, assembled ONCE and used twice: the page names these
	// lanes to the reader, and the readings below refuse to state exact figures
	// over them. Two derivations of one list could disagree about whether the
	// day was wholly seen.
	//
	// Both halves are needed, and missing the second half is the defect this
	// shape exists to prevent. `unavailable(day)` reads the assembled day's own
	// omitted lanes; the who-is-waiting and owed-leads sources are read BESIDE
	// that day and are absent from it. Those two are precisely what
	// `buyer_replies` and `prospecting` count, so leaving them out let a refused
	// lane print a confident zero — the one direction these figures must never
	// fail in.
	missing := unavailable(day)
	for _, refusal := range besideTheDay {
		if refusal != nil {
			missing = append(missing, *refusal)
		}
	}
	out := crmcontracts.Worklist{
		AsOf:  day.AsOf,
		Queue: ordered,
		// The headings, in draw order, over the rows this page actually holds.
		Bands: &bands,
		// The bar is re-derived rather than threaded out of classifyDay: it is a
		// pure function of the same day this call already holds, so the two
		// cannot disagree, and threading it would change a signature twenty-odd
		// callers spell.
		// The DAY, not this page: `considered` is every candidate weighed, and
		// the sentence's own `total` has always been that scope. Counting the
		// bands over `shown` printed one page's four figures beside the day's
		// fifth, in a sentence shaped like a breakdown of the total.
		Summary:            summarize(considered, materialBarOf(day, s.money)),
		SourcesUnavailable: missing,
		// `considered` is every candidate this read weighed, `shown` what
		// survived folding and the cut. Both are already in hand, so no figure
		// here costs a query that could disagree with the page it describes.
		Reach: reachOf(considered, shown, bounded),
		// The same accounting per kind of work. Both read the same two
		// snapshots, so the per-source and per-category figures are two views of
		// one answer rather than two answers.
		Counts: countsOf(considered, shown, bounded),
		// The outcome figures beside the per-kind tallies, over the same
		// snapshot: what the day's work is worth rather than how much of it
		// there is.
		//
		// It takes the SAME withheld list the page publishes, so the strip cannot
		// state a clear day over a lane this reader was refused. Sharing the value
		// also shares its DSR suppression, which is correct here rather than
		// merely convenient: DSR rows classify as `system` and feed none of the
		// four readings, so a suppressed DSR lane hides no work these figures
		// count. A lane whose rows DID feed one would have to be named.
		Readings: readingsOf(considered, bounded, missing),
	}
	if filter != "" {
		narrowed := crmcontracts.WorklistFilter(filter)
		out.Filter = &narrowed
	}
	// The token for the row this page stopped on, minted ONLY when there is more
	// behind it. A cursor on a final page invites one more request that can only
	// answer empty, and a client walking until the cursor disappears would never
	// stop.
	//
	// Fingerprinted with `s.taskOwner`, the same resolved owner the two
	// narrowing passes above read. Taking it from the request instead would let
	// the token and the rows it continues disagree about whose queue this is.
	//
	// The position is how far into THIS read's ranking the page got — where the
	// cut landed, not a running total of rows handed out. The two differ the
	// moment the day moves between pages, and a running total would push the
	// offset past the work still owed.
	if more && len(shown) > 0 {
		token := encodeCursor(reached, scope, filter, s.taskOwner, walk.id)
		out.NextCursor = &token
	}
	// What has happened to this walk since it started, where there is a walk to
	// say it about. A first page that froze nothing states nothing: the fields
	// would be a claim about a walk that does not exist.
	if state := walk.state(); state != nil {
		out.Walk = state
	}
	return out
}
