// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
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
	minted := encodeCursor(
		ranked{item: crmcontracts.WorklistItem{Source: "task", Id: "t1"}}, 1,
		scopeMine, "", ids.UUID{})

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
	minted := encodeCursor(
		ranked{item: crmcontracts.WorklistItem{Source: "task", Id: "t1"}}, 1,
		scopeMine, "", ids.UUID{})
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
		// Well-formed base64 of well-formed JSON that names no row: `{}`
		// decodes cleanly and would resume from nowhere.
		"e30",
		"eyJzIjoidGFzayJ9", // {"s":"task"} — a source with no row
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
	if cursor.Row != "" {
		t.Errorf("an absent cursor named row %q", cursor.Row)
	}
}

// Every source the queue can raise must survive a round trip. The claim in
// encodeCursor — that this shape cannot fail to encode — is only worth making
// if something checks it against the real vocabulary.
func TestACursorRoundTripsEveryRowShapeTheQueueCanCarry(t *testing.T) {
	for _, source := range []crmcontracts.WorklistItemSource{
		"task", "deal_at_risk", sourceWaiting, sourceLeadResponse,
		"approval", "bounce", "meeting_prep", "duplicate",
	} {
		row := ranked{item: crmcontracts.WorklistItem{Source: source, Id: "some-record-id"}}
		token := encodeCursor(row, 1, scopeMine, "", ids.UUID{})
		if token == "" {
			t.Fatalf("source %q minted an empty cursor", source)
		}
		back, err := decodeCursor(token, scopeMine, "", ids.UUID{})
		if err != nil {
			t.Fatalf("source %q did not survive a round trip: %v", source, err)
		}
		if back.Source != string(source) || back.Row != "some-record-id" {
			t.Errorf("source %q round-tripped as (%q,%q)", source, back.Source, back.Row)
		}
	}
}

// A vanished anchor must NOT end the walk. The position carries it, so the
// caller continues from where they had got to rather than being told, wrongly,
// that they had reached the end with work still owed.
func TestAVanishedAnchorDoesNotEndTheWalk(t *testing.T) {
	rows := []ranked{
		{item: crmcontracts.WorklistItem{Source: "task", Id: "a"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "b"}},
	}
	left := resume(rows, worklistCursor{Source: "task", Row: "answered-and-gone", Served: 1})
	if len(left) != 1 || left[0].item.Id != "b" {
		t.Errorf("a vanished anchor left %v; the position should have carried the walk to b",
			rowIDsOf(left))
	}
}

// The identity half wins when it is EARLIER than the position, which is what
// keeps a row that moved up from being stepped over.
//
// Here the anchor sits at index 1 because a row arrived above it, while the
// token says one row was served. Position-only would resume at 1 and hand back
// the anchor itself; identity-only would resume at 2 and skip the arrival. The
// earlier of the two — index 1 — repeats the anchor rather than losing a row,
// which is the trade this cursor makes deliberately.
func TestTheEarlierOfPositionAndIdentityWins(t *testing.T) {
	rows := []ranked{
		{item: crmcontracts.WorklistItem{Source: "task", Id: "arrived-since"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "the-anchor"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "still-owed"}},
	}
	left := resume(rows, worklistCursor{Source: "task", Row: "the-anchor", Served: 1})
	if len(left) != 2 || left[0].item.Id != "the-anchor" {
		t.Fatalf("resume returned %v, want the anchor and everything after it", rowIDsOf(left))
	}
}

// Id alone is the owning RECORD's id, which two sources can both point at — a
// deal that is quiet AND has a customer waiting. The pair is the identity.
//
// The anchor is the SECOND of the pair on purpose. Anchoring on the first would
// pass under either rule: a match on the id alone finds that same row, so the
// two implementations agree and the test proves nothing. Only a cursor naming
// the later one tells them apart — matching on the id alone resolves the anchor
// one row too early and hands back a row the caller already has.
func TestTwoSourcesSharingARecordIdAreDifferentRows(t *testing.T) {
	rows := []ranked{
		{item: crmcontracts.WorklistItem{Source: "deal_at_risk", Id: "same-deal"}},
		{item: crmcontracts.WorklistItem{Source: sourceWaiting, Id: "same-deal"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "still-owed"}},
	}
	left := resume(rows, worklistCursor{Source: sourceWaiting, Row: "same-deal", Served: 2})
	if len(left) != 1 || left[0].item.Id != "still-owed" {
		t.Errorf("resume returned %v; matching the id alone stops at the wrong row and repeats work",
			rowIDsOf(left))
	}
}

func rowIDsOf(rows []ranked) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.item.Id)
	}
	return out
}

func mustUUID(t *testing.T, s string) ids.UUID {
	t.Helper()
	parsed, err := ids.Parse(s)
	if err != nil {
		t.Fatalf("parsing the fixture uuid: %v", err)
	}
	return parsed
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

// A folded batch row must be a STABLE anchor across pages.
//
// Folding replaces a pile of alike decisions with one row, and that row is what
// a cursor can land on. Its id is derived from the group's key and cause
// (batchID), so the same pile folds to the same id on every read. If a later
// change gave a batch row a per-read identity — a counter, an instant, a random
// id — a cursor naming one would find nothing on the next page and the walk
// would end early with work still owed, silently.
func TestAFoldedRowIsAStableAnchorAcrossReads(t *testing.T) {
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
func TestALiveDayDoesNotLoseRowsBetweenPages(t *testing.T) {
	svc := &Service{}
	at := func(id string, due time.Duration) crmcontracts.AttentionItem {
		return item(id, "task", withDue(rankInstant.Add(due)))
	}
	page := func(day crmcontracts.Attention, cursor worklistCursor) crmcontracts.Worklist {
		return svc.worklistFrom(t.Context(), day, scopeAll, "", 1,
			waitingRead{}, leadRead{}, cursor)
	}
	shown := func(out crmcontracts.Worklist) []string {
		got := []string{}
		for _, row := range out.Queue {
			got = append(got, row.Id)
		}
		return got
	}

	t.Run("a row that moves up but stays behind the reader is still reached", func(t *testing.T) {
		// aa, bb, cc, dd by deadline. Page one hands out aa.
		before := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("aa", time.Hour), at("bb", 2*time.Hour),
			at("cc", 3*time.Hour), at("dd", 4*time.Hour),
		}}
		first := page(before, worklistCursor{})
		cursor, err := decodeCursor(*first.NextCursor, scopeAll, "", ids.UUID{})
		if err != nil {
			t.Fatalf("decoding page one's cursor: %v", err)
		}
		// dd overtakes bb and cc but still sorts behind the already-served aa.
		// A position-only resume would step over whatever landed on the old
		// offset; taking the earlier of position and identity keeps it.
		after := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("aa", time.Hour), at("bb", 2*time.Hour),
			at("cc", 3*time.Hour), at("dd", 90*time.Minute),
		}}
		reached := map[string]bool{}
		for _, id := range shown(first) {
			reached[id] = true
		}
		for range 8 {
			out := page(after, cursor)
			for _, id := range shown(out) {
				reached[id] = true
			}
			if out.NextCursor == nil {
				break
			}
			cursor, err = decodeCursor(*out.NextCursor, scopeAll, "", ids.UUID{})
			if err != nil {
				t.Fatalf("decoding a later cursor: %v", err)
			}
		}
		for _, want := range []string{"aa", "bb", "cc", "dd"} {
			if !reached[want] {
				t.Errorf("row %q was never shown to the reader; the walk stepped over it", want)
			}
		}
	})

	// The limit of ANY forward-only cursor over a live re-ranked set, pinned
	// here so it is a known property rather than a surprise.
	//
	// A row that overtakes rows the caller has already been handed now sorts
	// behind them. Returning it would mean re-serving that whole prefix, which
	// is the walk that never terminates. So this read does not show it, and the
	// next read of the day — the rep opening their queue again — leads with it.
	//
	// That loss is bounded and self-correcting, and it is the only one accepted
	// here. It is not the same as the defects the cases above cover, where a
	// row went missing from an unchanged part of the day.
	t.Run("a row that overtakes what the reader already has waits for the next read", func(t *testing.T) {
		before := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("aa", time.Hour), at("bb", 2*time.Hour), at("cc", 3*time.Hour),
		}}
		first := page(before, worklistCursor{})
		cursor, err := decodeCursor(*first.NextCursor, scopeAll, "", ids.UUID{})
		if err != nil {
			t.Fatalf("decoding page one's cursor: %v", err)
		}
		// cc becomes the most urgent thing on the day, ahead of the served aa.
		after := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("aa", time.Hour), at("bb", 2*time.Hour), at("cc", -5*time.Hour),
		}}
		out := page(after, cursor)
		if len(out.Queue) == 0 {
			t.Fatal("the walk ended although rows behind the anchor were still owed")
		}
		// A fresh read leads with it, so the row is delayed rather than lost.
		fresh := page(after, worklistCursor{})
		if len(fresh.Queue) == 0 || fresh.Queue[0].Id != "cc" {
			t.Errorf("a fresh read did not lead with the newly urgent row: %v", shown(fresh))
		}
	})

	t.Run("an anchor that slides to last does not end the walk", func(t *testing.T) {
		before := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("aa", time.Hour), at("bb", 2*time.Hour), at("cc", 3*time.Hour),
		}}
		first := page(before, worklistCursor{})
		cursor, err := decodeCursor(*first.NextCursor, scopeAll, "", ids.UUID{})
		if err != nil {
			t.Fatalf("decoding page one's cursor: %v", err)
		}
		// The anchor itself is deprioritised to the very end. An identity-only
		// resume returns nothing at all — "after aa" is empty — and the reader
		// is told their day is finished with two rows still owed.
		after := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("aa", 99*time.Hour), at("bb", 2*time.Hour), at("cc", 3*time.Hour),
		}}
		out := page(after, cursor)
		if len(out.Queue) == 0 {
			t.Fatal("the walk reported itself finished while two rows were still owed")
		}
	})

	t.Run("an anchor that vanishes does not end the walk", func(t *testing.T) {
		before := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("aa", time.Hour), at("bb", 2*time.Hour), at("cc", 3*time.Hour),
		}}
		first := page(before, worklistCursor{})
		cursor, err := decodeCursor(*first.NextCursor, scopeAll, "", ids.UUID{})
		if err != nil {
			t.Fatalf("decoding page one's cursor: %v", err)
		}
		// aa is answered between pages. The position carries the walk.
		after := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{
			at("bb", 2*time.Hour), at("cc", 3*time.Hour),
		}}
		out := page(after, cursor)
		if len(out.Queue) == 0 {
			t.Fatal("answering the anchor ended the walk; the rows behind it became unreachable")
		}
	})
}

// The two spellings of one question must fingerprint alike, or a walk breaks on
// a difference the caller cannot see. Both are cases the contract itself calls
// the same question.
func TestEquivalentSpellingsOfOneQuestionShareACursor(t *testing.T) {
	t.Parallel()

	t.Run("an omitted filter and filter=all", func(t *testing.T) {
		minted := encodeCursor(
			ranked{item: crmcontracts.WorklistItem{Source: "task", Id: "t1"}}, 1,
			scopeMine, "", ids.UUID{})
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
// less() ends at the id, so rows sharing one tie there and their (source, id)
// anchors cannot be told apart: resume() finds the first every time. The
// position half of the token is what carries the walk past them — without it a
// cursor naming the second would resolve to the first and the same page would
// be served forever.
func TestIndistinguishableAnchorsDoNotStallAWalk(t *testing.T) {
	rows := []ranked{
		{item: crmcontracts.WorklistItem{Source: "task", Id: "twin"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "twin"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "after"}},
	}
	// The caller has had both twins; the identity resolves to the first.
	left := resume(rows, worklistCursor{Source: "task", Row: "twin", Served: 2})
	if len(left) == len(rows) {
		t.Fatal("the walk made no progress; a cursor on a duplicated id would repeat forever")
	}
}

// Naming yourself and omitting the owner must return the SAME page.
//
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
