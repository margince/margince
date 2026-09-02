// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A day of `n` plain tasks, which classify into one level and therefore exercise
// the id tie-break that makes the ranking deterministic.
func tasksDay(n int) crmcontracts.Attention {
	planned := make([]crmcontracts.AttentionItem, 0, n)
	for i := range n {
		planned = append(planned, item(
			// Padded so string order and numeric order agree: less() breaks a
			// complete tie on the id, and "t10" < "t2" would make the walk
			// correct but unreadable when a failure prints it.
			"t"+string(rune('a'+i/26))+string(rune('a'+i%26)),
			"task",
			withDue(rankInstant.Add(time.Duration(i)*time.Minute)),
		))
	}
	return crmcontracts.Attention{AsOf: rankInstant, Planned: planned}
}

// Walking every page must yield every row exactly once. This is the acceptance
// criterion the whole cursor exists for: a paginated backlog that silently drops
// a page passes every unit test about a single page.
func TestAWalkReachesEveryRowExactlyOnce(t *testing.T) {
	const rows, perPage = 23, 5
	svc := &Service{}
	day := tasksDay(rows)

	seen := map[string]int{}
	cursor := worklistCursor{}
	pages := 0
	for {
		out := svc.worklistFrom(t.Context(), day, scopeAll, "", perPage,
			waitingRead{}, leadRead{}, cursor)
		pages++
		for _, row := range out.Queue {
			seen[string(row.Source)+"|"+row.Id]++
		}
		if out.NextCursor == nil {
			break
		}
		if pages > rows {
			t.Fatalf("the walk did not terminate: %d pages for %d rows", pages, rows)
		}
		decoded, err := decodeCursor(*out.NextCursor, scopeAll, "", ids.UUID{})
		if err != nil {
			t.Fatalf("the cursor this endpoint minted was refused on the way back in: %v", err)
		}
		cursor = decoded
	}

	if len(seen) != rows {
		t.Errorf("the walk reached %d distinct rows, the day held %d", len(seen), rows)
	}
	for id, times := range seen {
		if times != 1 {
			t.Errorf("row %s was handed out %d times; a walk must not repeat a row", id, times)
		}
	}
	if want := 5; pages != want {
		t.Errorf("walked %d pages of %d over %d rows, want %d", pages, perPage, rows, want)
	}
}

// The count the page reports and the rows the walk actually yields must agree.
// A cursor that pages correctly while `counts` says something else is two
// answers to one question.
func TestTheWalkReconcilesAgainstTheCountItReports(t *testing.T) {
	const rows, perPage = 17, 4
	svc := &Service{}
	day := tasksDay(rows)

	first := svc.worklistFrom(t.Context(), day, scopeAll, "", perPage,
		waitingRead{}, leadRead{}, worklistCursor{})
	considered := 0
	for _, count := range first.Counts {
		considered += count.Considered
	}

	walked := 0
	cursor := worklistCursor{}
	for {
		out := svc.worklistFrom(t.Context(), day, scopeAll, "", perPage,
			waitingRead{}, leadRead{}, cursor)
		walked += len(out.Queue)
		if out.NextCursor == nil {
			break
		}
		decoded, err := decodeCursor(*out.NextCursor, scopeAll, "", ids.UUID{})
		if err != nil {
			t.Fatalf("decoding the minted cursor: %v", err)
		}
		cursor = decoded
	}
	if walked != considered {
		t.Errorf("the walk yielded %d rows, `counts` reported %d considered", walked, considered)
	}
}

// The last page must NOT carry a cursor. A client that walks until the cursor
// disappears would otherwise never stop.
func TestTheFinalPageOffersNoCursor(t *testing.T) {
	svc := &Service{}
	out := svc.worklistFrom(t.Context(), tasksDay(3), scopeAll, "", 25,
		waitingRead{}, leadRead{}, worklistCursor{})
	if out.NextCursor != nil {
		t.Errorf("a page holding every row offered a cursor to nothing: %q", *out.NextCursor)
	}
}

// A token minted for one question must not silently continue into another.
// Page two of a rep's own tasks becoming the team's deals is the failure, and
// nothing in the response would have said so.
func TestACursorIsRefusedWhenTheQuestionChanged(t *testing.T) {
	someone := ids.UUID(mustUUID(t, "11111111-1111-4111-8111-111111111111"))
	minted := encodeCursor(1, scopeMine, "", ids.UUID{})

	for _, tc := range []struct {
		name          string
		scope, filter string
		owner         ids.UUID
	}{
		{"a wider scope", scopeAll, "", ids.UUID{}},
		{"a different filter", scopeMine, "deals_at_risk", ids.UUID{}},
		{"somebody else's queue", scopeMine, "", someone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeCursor(minted, tc.scope, tc.filter, tc.owner)
			var mismatch *storekit.CursorSortMismatchError
			if !errors.As(err, &mismatch) {
				t.Errorf("continuing into %s was allowed (err=%v); it must be refused", tc.name, err)
			}
		})
	}
}

// Changing `limit` mid-walk is legitimate: it decides how many rows a page
// carries, not which rows exist. Refusing it would be a lie about what changed.
func TestChangingTheLimitDoesNotInvalidateACursor(t *testing.T) {
	minted := encodeCursor(1, scopeMine, "", ids.UUID{})
	if _, err := decodeCursor(minted, scopeMine, "", ids.UUID{}); err != nil {
		t.Errorf("a cursor was refused although only the page size changed: %v", err)
	}
}

// A token this endpoint never minted is the caller's mistake, not a server
// fault, and it must not read as "start from the top" — a walk that silently
// restarts is the shape that never terminates.
func TestATokenThisEndpointDidNotMintIsRefused(t *testing.T) {
	for _, token := range []string{
		"not-base64-at-all!!",
		// Well-formed base64 of well-formed JSON carrying no fingerprint: `{}`
		// decodes cleanly and leaves a zero offset, which would restart a walk
		// for a question nobody minted a token for.
		"e30",
		"bnVsbA", // `null`, which unmarshals to the zero value just as cleanly
	} {
		_, err := decodeCursor(token, scopeMine, "", ids.UUID{})
		if err == nil {
			t.Errorf("token %q was accepted; a token nobody minted must be refused", token)
		}
	}
}

// An empty token is the start of the walk rather than a fault, which is what
// lets one code path serve both the first page and the rest.
func TestNoCursorStartsAtTheTop(t *testing.T) {
	cursor, err := decodeCursor("", scopeMine, "", ids.UUID{})
	if err != nil {
		t.Fatalf("an absent cursor was treated as a fault: %v", err)
	}
	if cursor.At != 0 {
		t.Errorf("an absent cursor named offset %d", cursor.At)
	}
}

// A cursor minted while reading one person's queue must not open another's.
//
// This is the end-to-end shape of the fingerprint rule, driven through
// Worklist rather than decodeCursor: the mint reads `s.taskOwner` off the
// narrowed service and the decode reads `namedOwner` off the resolver, and a
// test that only exercises decodeCursor would never notice those two coming
// apart. If they did, a manager's page-two token would silently open a
// different rep's day under the first rep's heading.
func TestACursorFromOneQueueCannotOpenAnothers(t *testing.T) {
	t.Parallel()

	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	// Three waits each, so a page of two leaves a third behind and the read
	// actually mints a cursor.
	waits := []WaitingCustomer{}
	for i := range 3 {
		waits = append(waits,
			WaitingCustomer{
				ActivityID: ids.NewV7(), Subject: "the rep's customer",
				Since: readInstant.Add(-time.Duration(i+2) * 24 * time.Hour), OwnerID: theRep,
			},
			WaitingCustomer{
				ActivityID: ids.NewV7(), Subject: "the manager's own customer",
				Since: readInstant.Add(-time.Duration(i+2) * 24 * time.Hour), OwnerID: theManager,
			})
	}
	svc.waiting = waitingOwnedBy(waits)
	svc.teammates = teammatesSaying(true)

	first, err := svc.Worklist(managerReading(), "", "", theRep, 2, "")
	if err != nil {
		t.Fatalf("opening the rep's queue: %v", err)
	}
	if first.NextCursor == nil {
		t.Fatal("a page of two over three rows offered no cursor; the walk cannot continue")
	}

	// The same token, carried onto the manager's own queue.
	_, err = svc.Worklist(managerReading(), "", "", ids.UUID{}, 2, *first.NextCursor)
	if err == nil {
		t.Error("a cursor minted on the rep's queue was accepted on the manager's own; " +
			"page two would answer about a different person with nothing saying so")
	}

	// And it still works on the queue it was minted for.
	if _, err := svc.Worklist(managerReading(), "", "", theRep, 2, *first.NextCursor); err != nil {
		t.Errorf("the cursor was refused on the very queue it was minted for: %v", err)
	}
}

// Folding must be DETERMINISTIC, because an offset means nothing over a
// ranking that reshapes itself between reads.
//
// Folding replaces a pile of alike decisions with one row, so it changes how
// many rows the ranking holds and therefore what every offset after it points
// at. Its id comes from the group's key and cause (batchID), and the group's
// membership from the day itself, so one day folds the same way on every read.
// A batch row given a per-read identity — a counter, an instant, a random id —
// would not break this test's assertion by itself, but it is the tell that the
// fold had become read-dependent, and a walk over a set that reshapes under it
// silently skips or repeats.
func TestFoldingIsDeterministicAcrossReads(t *testing.T) {
	t.Parallel()

	// A pile deep enough to fold, of the routine contact decisions that fold.
	needs := make([]crmcontracts.AttentionItem, 0, 12)
	for i := range 12 {
		needs = append(needs, item(
			"d"+string(rune('a'+i)), "approval", withKind("capture_counterparty")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: needs}

	svc := &Service{}
	first := svc.worklistFrom(t.Context(), day, scopeAll, "", 25,
		waitingRead{}, leadRead{}, worklistCursor{})
	second := svc.worklistFrom(t.Context(), day, scopeAll, "", 25,
		waitingRead{}, leadRead{}, worklistCursor{})

	batched := func(out crmcontracts.Worklist) []string {
		ids := []string{}
		for _, row := range out.Queue {
			if row.Batch != nil {
				ids = append(ids, string(row.Source)+"|"+row.Id)
			}
		}
		return ids
	}
	one, two := batched(first), batched(second)
	if len(one) == 0 {
		t.Fatal("the fixture folded nothing; this test would pass vacuously")
	}
	if len(one) != len(two) {
		t.Fatalf("two reads of one day folded differently: %v against %v", one, two)
	}
	for i := range one {
		if one[i] != two[i] {
			t.Errorf("a folded row changed identity between reads: %q then %q; "+
				"a cursor naming it would find nothing and end the walk early", one[i], two[i])
		}
	}
}

// A day that moves between pages must not lose a row.
//
// These two are the reason the token carries a position AND an identity. Each
// half alone fails one of them, in opposite directions, and the failure is
// silent either way: the reader is simply never shown work they are owed.
// The two spellings of one question must fingerprint alike, or a walk breaks on
// a difference the caller cannot see. Both are cases the contract itself calls
// the same question.
func TestEquivalentSpellingsOfOneQuestionShareACursor(t *testing.T) {
	t.Parallel()

	t.Run("an omitted filter and filter=all", func(t *testing.T) {
		minted := encodeCursor(1, scopeMine, "", ids.UUID{})
		if _, err := decodeCursor(minted, scopeMine, "all", ids.UUID{}); err != nil {
			t.Errorf("a client sending its documented `all` default on page two was refused: %v", err)
		}
	})

	t.Run("an omitted owner and naming yourself", func(t *testing.T) {
		svc := NewService(
			stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
			stubBriefing{}, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
		waits := []WaitingCustomer{}
		for i := range 3 {
			waits = append(waits, WaitingCustomer{
				ActivityID: ids.NewV7(), Subject: "a customer",
				Since: readInstant.Add(-time.Duration(i+2) * 24 * time.Hour), OwnerID: theManager,
			})
		}
		svc.waiting = waitingOwnedBy(waits)
		svc.teammates = teammatesSaying(true)

		first, err := svc.Worklist(managerReading(), "", "", ids.UUID{}, 2, "")
		if err != nil {
			t.Fatalf("reading own queue: %v", err)
		}
		if first.NextCursor == nil {
			t.Fatal("a page of two over three rows offered no cursor")
		}
		// The same reader, naming themselves — which the contract calls the
		// question the default already answers.
		if _, err := svc.Worklist(managerReading(), "", "", theManager, 2, *first.NextCursor); err != nil {
			t.Errorf("naming yourself on page two was refused as a different question: %v", err)
		}
	})
}

// Two rows that are indistinguishable as anchors must not stall a walk.
//
// less() ends at the record id, so rows sharing one tie there and their
// (source, id) anchors cannot be told apart. An anchor search that took the
// FIRST match would resolve to the same index however many twins the caller had
// already been handed, and the same page would be served for as long as the
// client kept asking — the one shape here that never terminates.
//
// Driven as a real walk rather than a single resumeAt call. A stall only shows
// as a walk that does not end, so a test that steps once cannot see it, which
// is how an earlier version of this passed while proving nothing.
// resolveOwner collapses the two onto one resolved question, which changes
// which branch the read takes: a self-named owner used to go through
// forOwner/TasksOwnedBy and now falls through to forReader/TasksMine. Those
// two are equivalent for this case by construction — one files the query under
// actor.UserID, the other under an owner that IS actor.UserID — but "by
// construction" is the kind of claim that stops being true quietly, so it is
// checked against the assembled page rather than argued.
func TestNamingYourselfAnswersTheSamePageAsOmittingTheOwner(t *testing.T) {
	t.Parallel()

	build := func() *Service {
		svc := NewService(
			stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
			stubBriefing{}, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
		svc.waiting = waitingOwnedBy{
			{
				ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000b1"),
				Subject:    "their own customer",
				Since:      readInstant.Add(-3 * 24 * time.Hour), OwnerID: theManager,
			},
			{
				ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000b2"),
				Subject:    "somebody else's customer",
				Since:      readInstant.Add(-2 * 24 * time.Hour), OwnerID: theRep,
			},
			{
				// The row the two narrowings disagree about, and the reason
				// this fixture is not just two owned rows. keepReadersOwn keeps
				// a row nobody owns — the contract says an unowned customer
				// writing in is everybody's until somebody takes them — while
				// keepOwnedBy drops it. Without this row both spellings agree
				// however resolveOwner behaves, and the test proves nothing
				// about the change it exists to check.
				ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000b3"),
				Subject:    "a customer nobody owns",
				Since:      readInstant.Add(-4 * 24 * time.Hour),
			},
		}
		svc.teammates = teammatesSaying(true)
		return svc
	}

	omitted, err := build().Worklist(managerReading(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("reading with the owner omitted: %v", err)
	}
	named, err := build().Worklist(managerReading(), "", "", theManager, 25, "")
	if err != nil {
		t.Fatalf("reading with the owner named as yourself: %v", err)
	}

	rows := func(out crmcontracts.Worklist) []string {
		got := []string{}
		for _, row := range out.Queue {
			got = append(got, string(row.Source)+"|"+row.Id)
		}
		return got
	}
	left, right := rows(omitted), rows(named)
	if len(left) == 0 {
		t.Fatal("both reads were empty; this test would pass vacuously")
	}
	if len(left) != len(right) {
		t.Fatalf("two spellings of one question answered differently: %v against %v", left, right)
	}
	for i := range left {
		if left[i] != right[i] {
			t.Errorf("row %d differs between the two spellings: %q against %q", i, left[i], right[i])
		}
	}
	if omitted.Scope != named.Scope {
		t.Errorf("the two spellings reported different scopes: %q against %q", omitted.Scope, named.Scope)
	}
}

// A crafted token must be refused, never crash the request.
//
// The cursor is client-supplied and unsigned, so a caller can hand back any
// position they like. An unchecked negative reaches `rows[n:]`, which panics —
// a denial of service on an authenticated endpoint, reachable by hand.
func TestACursorNamingAPositionBeforeTheFirstRowIsRefused(t *testing.T) {
	t.Parallel()

	forged := base64.RawURLEncoding.EncodeToString([]byte(
		`{"s":"task","r":"t1","n":-1,"p":"` + fingerprint(scopeMine, "", ids.UUID{}) + `"}`))

	cursor, err := decodeCursor(forged, scopeMine, "", ids.UUID{})
	if err == nil {
		t.Fatalf("a negative position was accepted as %+v; it must be refused", cursor)
	}
	var malformed *storekit.MalformedCursorError
	if !errors.As(err, &malformed) {
		t.Errorf("a negative position answered %v; it is the caller's mistake and must be "+
			"malformed_cursor", err)
	}
}

func mustUUID(t *testing.T, s string) ids.UUID {
	t.Helper()
	parsed, err := ids.Parse(s)
	if err != nil {
		t.Fatalf("parsing the fixture uuid: %v", err)
	}
	return parsed
}

// A walk always terminates, whatever the day does between pages.
//
// The offset strictly increases and the candidate set is bounded, so there is
// no arrangement of arrivals, answers and reprioritisations that makes a client
// paging to exhaustion loop. That is the property the identity anchor could not
// give: with one, a row answered between pages emptied the remainder and a
// duplicated record id resolved to the same index forever.
func TestAWalkTerminatesHoweverTheDayMoves(t *testing.T) {
	svc := &Service{}
	at := func(id string, d time.Duration) crmcontracts.AttentionItem {
		return item(id, "task", withDue(rankInstant.Add(d)))
	}
	// Each read hands back a differently ordered day, including one where every
	// row is new and one where the set shrinks under the caller's feet.
	days := []crmcontracts.Attention{
		{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("aa", time.Hour), at("bb", 2*time.Hour), at("cc", 3*time.Hour),
		}},
		{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("cc", -9*time.Hour), at("aa", 5*time.Hour), at("bb", 6*time.Hour),
		}},
		{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("zz", time.Hour), at("yy", 2*time.Hour),
		}},
		{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{}},
	}

	cursor := worklistCursor{}
	for page := range 20 {
		out := svc.worklistFrom(t.Context(), days[min(page, len(days)-1)], scopeAll, "", 1,
			waitingRead{}, leadRead{}, cursor)
		if out.NextCursor == nil {
			return
		}
		decoded, err := decodeCursor(*out.NextCursor, scopeAll, "", ids.UUID{})
		if err != nil {
			t.Fatalf("decoding a cursor mid-walk: %v", err)
		}
		if decoded.At <= cursor.At {
			t.Fatalf("the offset did not advance: %d then %d; a walk that does not "+
				"move forward is one that never ends", cursor.At, decoded.At)
		}
		cursor = decoded
	}
	t.Fatal("the walk did not terminate in 20 pages over days of at most three rows")
}

// The cost of an offset, pinned so it is a known property rather than a
// surprise found in production.
//
// The ranking is rebuilt on every read. A row that crosses the page boundary
// between two reads is served twice or not at all ON THIS WALK. It is not lost
// from the product — the next read ranks it afresh and shows it — and that
// bounded, self-correcting delay is what buys a walk that cannot report itself
// finished while work is still owed.
func TestARowCrossingThePageBoundaryWaitsForTheNextRead(t *testing.T) {
	svc := &Service{}
	at := func(id string, d time.Duration) crmcontracts.AttentionItem {
		return item(id, "task", withDue(rankInstant.Add(d)))
	}
	before := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
		at("aa", time.Hour), at("bb", 2*time.Hour), at("cc", 3*time.Hour),
	}}
	first := svc.worklistFrom(t.Context(), before, scopeAll, "", 1,
		waitingRead{}, leadRead{}, worklistCursor{})
	if len(first.Queue) != 1 || first.Queue[0].Id != "aa" {
		t.Fatalf("page one handed back %v, want aa", queueIDsOf(first))
	}
	cursor, err := decodeCursor(*first.NextCursor, scopeAll, "", ids.UUID{})
	if err != nil {
		t.Fatalf("decoding page one: %v", err)
	}

	// cc becomes the most urgent row, crossing above the boundary the caller
	// has already passed.
	after := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
		at("aa", time.Hour), at("bb", 2*time.Hour), at("cc", -5*time.Hour),
	}}
	second := svc.worklistFrom(t.Context(), after, scopeAll, "", 1,
		waitingRead{}, leadRead{}, cursor)
	if len(second.Queue) == 0 {
		t.Fatal("the walk ended with rows still owed")
	}

	// The delay is bounded: a fresh read leads with it.
	fresh := svc.worklistFrom(t.Context(), after, scopeAll, "", 1,
		waitingRead{}, leadRead{}, worklistCursor{})
	if len(fresh.Queue) == 0 || fresh.Queue[0].Id != "cc" {
		t.Errorf("a fresh read did not lead with the newly urgent row: %v", queueIDsOf(fresh))
	}
}

// A cursor round-trips the position it was minted at, unchanged.
func TestACursorRoundTripsThePositionItWasMintedAt(t *testing.T) {
	t.Parallel()

	for _, at := range []int{0, 1, 25, 100, 4096} {
		token := encodeCursor(at, scopeMine, "", ids.UUID{})
		if token == "" {
			t.Fatalf("offset %d minted an empty cursor", at)
		}
		back, err := decodeCursor(token, scopeMine, "", ids.UUID{})
		if err != nil {
			t.Fatalf("offset %d did not survive a round trip: %v", at, err)
		}
		if back.At != at {
			t.Errorf("offset %d round-tripped as %d", at, back.At)
		}
	}
}

func queueIDsOf(out crmcontracts.Worklist) []string {
	got := []string{}
	for _, row := range out.Queue {
		got = append(got, row.Id)
	}
	return got
}

// A token carrying no fingerprint must be refused, not read as "start again".
//
// `{}` and `null` both decode cleanly into the zero cursor, whose offset is 0.
// Without this guard they would be accepted as a legitimate first page — a
// walk restarting for a question nobody minted a token for, and the one reading
// under which a forged token does something rather than nothing.
func TestACursorWithNoFingerprintIsRefused(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{`{}`, `null`, `{"n":3}`} {
		token := base64.RawURLEncoding.EncodeToString([]byte(raw))
		_, err := decodeCursor(token, scopeMine, "", ids.UUID{})
		if err == nil {
			t.Errorf("%s was accepted as a cursor; it names no question and must be refused", raw)
			continue
		}
		var malformed *storekit.MalformedCursorError
		if !errors.As(err, &malformed) {
			t.Errorf("%s answered %v; a token nobody minted is malformed_cursor", raw, err)
		}
	}
}

// An offset past the end of the day must end the walk, not reach past the slice.
//
// The token is client-supplied, so an offset larger than the day now holds is
// reachable both by hand and by an honest client whose tail was dealt with
// between pages. Either way it is a finished walk, not a fault — and without the
// clamp it is `rows[n:]` on a short slice, which panics.
func TestAnOffsetPastTheEndEndsTheWalk(t *testing.T) {
	t.Parallel()

	rows := []ranked{
		{item: crmcontracts.WorklistItem{Source: "task", Id: "a"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "b"}},
	}
	if at := resumeAt(rows, worklistCursor{At: 99, Params: "x"}); at != len(rows) {
		t.Errorf("an offset of 99 over 2 rows resumed at %d; it must clamp to %d",
			at, len(rows))
	}
	// And through the page cut, where the panic would actually happen.
	shown, more, _ := pageFrom(rows, 25, worklistCursor{At: 99, Params: "x"})
	if len(shown) != 0 || more {
		t.Errorf("an offset past the end returned %d rows (more=%v); the walk is over",
			len(shown), more)
	}
}

// The offset a cursor carries is where the CUT landed in this read's ranking.
//
// `reached` and "the incoming offset plus the rows served" happen to agree
// wherever a cursor is actually minted, and that is worth knowing rather than
// relying on: a token is minted only when rows remain past the cut, which needs
// a full `limit` page, and a clamped read has nothing remaining and mints
// nothing. So the two formulas cannot diverge here — but `reached` is the one
// that stays right if that ever stops holding, because it reads the ranking in
// front of it rather than trusting what the caller said about a previous one.
func TestTheOffsetIsWhereTheCutLandedInThisRanking(t *testing.T) {
	svc := &Service{}

	// A cursor whose offset the day can no longer honour: the tail was dealt
	// with between reads, so the clamp pulls the read back to the end.
	out, more, reached := pageFrom(
		[]ranked{{item: crmcontracts.WorklistItem{Source: "task", Id: "only"}}},
		25, worklistCursor{At: 10, Params: "x"})
	if reached != 1 {
		t.Errorf("the cut landed at %d over a single row; it must be a position in THIS "+
			"ranking, so it can never point past the rows that exist", reached)
	}
	if len(out) != 0 || more {
		t.Errorf("expected an empty final page, got %d rows (more=%v)", len(out), more)
	}

	// End to end: a walk advances by exactly the page size, and the offset it
	// publishes indexes the ranking rather than counting the caller's history.
	first := svc.worklistFrom(t.Context(), tasksDay(5), scopeAll, "", 2,
		waitingRead{}, leadRead{}, worklistCursor{})
	cursor, err := decodeCursor(*first.NextCursor, scopeAll, "", ids.UUID{})
	if err != nil {
		t.Fatalf("decoding page one: %v", err)
	}
	if cursor.At != 2 {
		t.Errorf("a first page of two advanced the offset to %d, want 2", cursor.At)
	}
	second := svc.worklistFrom(t.Context(), tasksDay(5), scopeAll, "", 2,
		waitingRead{}, leadRead{}, cursor)
	next, err := decodeCursor(*second.NextCursor, scopeAll, "", ids.UUID{})
	if err != nil {
		t.Fatalf("decoding page two: %v", err)
	}
	if next.At != 4 {
		t.Errorf("page two advanced the offset to %d, want 4", next.At)
	}
}
