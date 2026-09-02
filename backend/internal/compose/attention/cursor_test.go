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
// the id tie-break that makes the ranking total.
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
		ranked{item: crmcontracts.WorklistItem{Source: "task", Id: "t1"}},
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
		ranked{item: crmcontracts.WorklistItem{Source: "task", Id: "t1"}},
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
		token := encodeCursor(row, scopeMine, "", ids.UUID{})
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

// A row that vanishes between pages ends the walk. Restarting from the top
// instead would hand a client paging to exhaustion the same first page forever,
// each pass looking like ordinary progress.
func TestAWalkEndsWhenItsAnchorIsGone(t *testing.T) {
	rows := []ranked{
		{item: crmcontracts.WorklistItem{Source: "task", Id: "a"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "b"}},
	}
	left := resume(rows, worklistCursor{Source: "task", Row: "answered-and-gone"})
	if len(left) != 0 {
		t.Errorf("a vanished anchor left %d rows; the walk must end rather than restart", len(left))
	}
}

// The anchor is found by identity, never by position. Positions move whenever
// the day moves, so an index would resume at whatever row slid into that slot —
// skipping work silently, which is the direction this queue must never fail in.
func TestTheAnchorIsFoundByIdentityNotPosition(t *testing.T) {
	rows := []ranked{
		{item: crmcontracts.WorklistItem{Source: "task", Id: "arrived-since"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "the-anchor"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "still-owed"}},
	}
	left := resume(rows, worklistCursor{Source: "task", Row: "the-anchor"})
	if len(left) != 1 || left[0].item.Id != "still-owed" {
		t.Fatalf("resume returned %v, want exactly the row after the anchor", rowIDsOf(left))
	}
}

// Id alone is the owning RECORD's id, which two sources can both point at — a
// deal that is quiet AND has a customer waiting. The pair is the identity.
//
// The anchor is the SECOND of the pair on purpose. Anchoring on the first would
// pass under either rule: a match on the id alone finds that same row, so the
// two implementations agree and the test proves nothing. Only a cursor naming
// the later one tells them apart — matching on the id alone stops at the
// earlier row and hands back work the caller has already been given.
func TestTwoSourcesSharingARecordIdAreDifferentRows(t *testing.T) {
	rows := []ranked{
		{item: crmcontracts.WorklistItem{Source: "deal_at_risk", Id: "same-deal"}},
		{item: crmcontracts.WorklistItem{Source: sourceWaiting, Id: "same-deal"}},
		{item: crmcontracts.WorklistItem{Source: "task", Id: "still-owed"}},
	}
	left := resume(rows, worklistCursor{Source: sourceWaiting, Row: "same-deal"})
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
