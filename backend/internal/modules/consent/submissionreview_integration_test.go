// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Deciding what a subject proposed.
//
// The table has carried resolution, resolved_at and resolved_by since it
// shipped and nothing ever wrote them: a subject could type a correction into
// the link we mailed them, the row was filed, and no colleague could list it,
// see it, or act on it. Every correction anybody has ever sent is still sitting
// there unanswered.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordingApplier stands in for contacts' update path and records what it was
// asked to write.
type recordingApplier struct {
	calls  int
	field  string
	value  string
	refuse error
}

func (a *recordingApplier) ApplyCorrection(
	_ context.Context, _ ids.ContactID, field, value string,
) error {
	a.calls++
	a.field, a.value = field, value
	return a.refuse
}

// proposeCorrection sends one through the real public door, so the row under
// test is the row production writes.
func proposeCorrection(t *testing.T, e *channelConsentEnv, field, value string) ids.UUID {
	t.Helper()
	seedMarketingPurpose(t, e)
	link := issueLink(t, e)
	if _, err := e.store.SubmitConfirmation(e.ctx, link.Token, ConfirmSubmission{
		Corrections: map[string]string{field: value},
	}); err != nil {
		t.Fatalf("submitting the correction: %v", err)
	}
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM contact_confirm_submission WHERE contact_id = $1 ORDER BY submitted_at DESC LIMIT 1`,
		e.contact).Scan(&id); err != nil {
		t.Fatalf("reading the staged proposal: %v", err)
	}
	return id
}

// reviewerCtx is a seat that may open and edit the contact, which is what
// deciding about a proposal asks for.
func reviewerCtx(e *channelConsentEnv) context.Context {
	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{
		"contact": {Read: true, Update: true},
	}
	return principal.WithActor(e.ctx, actor)
}

// TestAProposalWaitsInTheQueueUntilSomebodyDecides is the gap this closes.
func TestAProposalWaitsInTheQueueUntilSomebodyDecides(t *testing.T) {
	e := setupChannelConsent(t)
	proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	unresolved := false
	queue, _, err := e.store.ListSubmissions(reviewerCtx(e), ListSubmissionsInput{Resolved: &unresolved})
	if err != nil {
		t.Fatalf("listing the queue: %v", err)
	}
	if len(queue) != 1 {
		t.Fatalf("the queue holds %d proposal(s), want 1 — a correction nobody can list is one "+
			"nobody can answer", len(queue))
	}
	got := queue[0]
	if got.Resolution != nil {
		t.Errorf("a fresh proposal reports resolution %v, want none", *got.Resolution)
	}
	if got.ProposedValue == nil || *got.ProposedValue != "Corrected Name" {
		t.Errorf("the queue shows %v, and a correction is only reviewable as a comparison",
			got.ProposedValue)
	}
}

// TestAnAcceptedCorrectionReachesTheRecordThroughTheOrdinaryPath.
//
// A writer of its own would be a second way to change a contact, answerable to
// none of the gates the first one is.
func TestAnAcceptedCorrectionReachesTheRecordThroughTheOrdinaryPath(t *testing.T) {
	e := setupChannelConsent(t)
	applier := &recordingApplier{}
	e.store = e.store.WithCorrectionApplier(applier)
	id := proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	out, err := e.store.ResolveSubmission(reviewerCtx(e), id,
		ResolveSubmissionInput{Resolution: SubmissionAccepted})
	if err != nil {
		t.Fatalf("accepting the correction: %v", err)
	}
	if applier.calls != 1 {
		t.Fatalf("the accepted correction reached the record %d time(s), want 1", applier.calls)
	}
	if applier.field != ConfirmFieldFullName || applier.value != "Corrected Name" {
		t.Errorf("wrote %s = %q, want the field and value the subject proposed",
			applier.field, applier.value)
	}
	if out.Resolution == nil || *out.Resolution != SubmissionAccepted {
		t.Errorf("the decision was not recorded: %v", out.Resolution)
	}
	if out.ResolvedBy == nil || *out.ResolvedBy == "" {
		t.Error("the decision names nobody, and who decided is half the record")
	}
}

// TestARejectedCorrectionChangesNothingAndSaysWhy.
//
// The proposal stays on the record: the subject asked, and an export showing
// the request without its answer would be telling half of it.
func TestARejectedCorrectionChangesNothingAndSaysWhy(t *testing.T) {
	e := setupChannelConsent(t)
	applier := &recordingApplier{}
	e.store = e.store.WithCorrectionApplier(applier)
	id := proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	out, err := e.store.ResolveSubmission(reviewerCtx(e), id, ResolveSubmissionInput{
		Resolution: SubmissionRejected,
		Note:       "The passport we hold spells it the other way.",
	})
	if err != nil {
		t.Fatalf("rejecting the correction: %v", err)
	}
	if applier.calls != 0 {
		t.Errorf("a rejected correction still wrote to the record %d time(s)", applier.calls)
	}
	if out.Note == nil || *out.Note == "" {
		t.Error(`the rejection records no reason — "we did not change it" is the whole of what ` +
			"the record says otherwise, and the subject who asked is entitled to more")
	}
	if out.ProposedValue == nil {
		t.Error("the rejected proposal lost the subject's own words")
	}
}

// TestASecondReviewerDoesNotOverwriteTheFirstsDecision.
//
// A stale screen pressed twice is not two decisions, and the first reviewer's
// name is the one that belongs on the row.
func TestASecondReviewerDoesNotOverwriteTheFirstsDecision(t *testing.T) {
	e := setupChannelConsent(t)
	applier := &recordingApplier{}
	e.store = e.store.WithCorrectionApplier(applier)
	id := proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	first, err := e.store.ResolveSubmission(reviewerCtx(e), id,
		ResolveSubmissionInput{Resolution: SubmissionAccepted})
	if err != nil {
		t.Fatalf("the first decision: %v", err)
	}
	second, err := e.store.ResolveSubmission(reviewerCtx(e), id,
		ResolveSubmissionInput{Resolution: SubmissionRejected, Note: "Actually no."})
	if err != nil {
		t.Fatalf("the second press: %v", err)
	}
	if second.Resolution == nil || *second.Resolution != SubmissionAccepted {
		t.Errorf("the second press rewrote the decision to %v", second.Resolution)
	}
	if second.ResolvedAt == nil || first.ResolvedAt == nil || !second.ResolvedAt.Equal(*first.ResolvedAt) {
		t.Error("the second press moved the timestamp, so the record says the decision was " +
			"made later than it was")
	}
	if applier.calls != 1 {
		t.Errorf("the record was written %d time(s) for one decision", applier.calls)
	}
}

// TestAnUnwritableFieldIsRefusedRatherThanSilentlyAccepted.
//
// Email and phone live on satellite tables the ordinary update path does not
// reach. An accept that changed nothing is the one outcome nobody could act on:
// the subject would be told their correction was taken and the record would
// still disagree.
func TestAnUnwritableFieldIsRefusedRatherThanSilentlyAccepted(t *testing.T) {
	e := setupChannelConsent(t)
	applier := &recordingApplier{refuse: ErrUnsupportedField}
	e.store = e.store.WithCorrectionApplier(applier)
	id := proposeCorrection(t, e, ConfirmFieldEmail, "new@example.test")

	_, err := e.store.ResolveSubmission(reviewerCtx(e), id,
		ResolveSubmissionInput{Resolution: SubmissionAccepted})
	if !errors.Is(err, ErrUnsupportedField) {
		t.Fatalf("accepting an unwritable correction answered %v, want ErrUnsupportedField", err)
	}

	// AND THE DECISION IS NOT RECORDED. A row reading "accepted" whose value
	// never reached the contact is worse than no row at all.
	var resolution *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT resolution FROM contact_confirm_submission WHERE id = $1`, id).Scan(&resolution); err != nil {
		t.Fatalf("reading the proposal: %v", err)
	}
	if resolution != nil {
		t.Errorf("the proposal reads %q after a refused accept", *resolution)
	}
}

// TestAReviewerWhoCannotEditTheContactDecidesNothing. Deciding what a proposal
// does to a record is a write to that record, even a decline.
func TestAReviewerWhoCannotEditTheContactDecidesNothing(t *testing.T) {
	e := setupChannelConsent(t)
	id := proposeCorrection(t, e, ConfirmFieldFullName, "Corrected Name")

	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{"contact": {Read: true}}
	readOnly := principal.WithActor(e.ctx, actor)

	if _, err := e.store.ResolveSubmission(readOnly, id,
		ResolveSubmissionInput{Resolution: SubmissionRejected}); err == nil {
		t.Fatal("a reader with no update grant recorded a decision about somebody's record")
	}
}

// TestTheQueueCarriesBothHalvesOfTheComparison is Codex's finding, and the
// screen it feeds could not make a decision without it.
//
// "She says Schmidt, we hold Schmitt" is the decision; the proposal alone is
// not. And a queue spanning every contact needs the name: two contacts
// proposing the same value on the same day are indistinguishable, while
// accepting either changes a different record.
func TestTheQueueCarriesBothHalvesOfTheComparison(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET full_name = 'Anna Schmitt' WHERE id = $1`, e.contact); err != nil {
		t.Fatalf("naming the contact: %v", err)
	}
	proposeCorrection(t, e, ConfirmFieldFullName, "Anna Schmidt")

	queue, _, err := e.store.ListSubmissions(reviewerCtx(e), ListSubmissionsInput{})
	if err != nil {
		t.Fatalf("listing the queue: %v", err)
	}
	if len(queue) != 1 {
		t.Fatalf("the queue holds %d proposal(s), want 1", len(queue))
	}
	got := queue[0]
	if got.ContactName != "Anna Schmitt" {
		t.Errorf("the row names %q, and a reviewer cannot tell whose record they are "+
			"changing without it", got.ContactName)
	}
	if got.CurrentValue != "Anna Schmitt" {
		t.Errorf("the row says the record currently holds %q — without it the reviewer sees "+
			"one half of a comparison and has to go and look up the other", got.CurrentValue)
	}
}

// ─── Paging ────────────────────────────────────────────────────────────────

// PROPOSALS BEYOND ONE PAGE ARE REACHABLE AT ALL, which is the gap this closes.
//
// The bound was the whole of the answer: past it the route returned the first
// page and said nothing about the rest, so the panel drew a complete-looking
// queue and the submissions that fell off the end were the NEWEST — the ones a
// reviewer has least chance of hearing about another way. No error, no count,
// nothing to press.
func TestEveryProposalIsReachableAcrossThePagesOfTheQueue(t *testing.T) {
	e := setupChannelConsent(t)
	const sent = 5
	filed := map[ids.UUID]bool{}
	for i := range sent {
		filed[proposeCorrection(t, e, ConfirmFieldFullName, fmt.Sprintf("Name %d", i))] = false
	}

	seen := map[ids.UUID]int{}
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > sent {
			t.Fatal("the walk did not end: a page that always says there is another is a loop, " +
				"not a queue")
		}
		batch, page, err := e.store.ListSubmissions(reviewerCtx(e), ListSubmissionsInput{
			Limit: 2, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		for _, sub := range batch {
			seen[sub.ID]++
		}
		if !page.HasMore {
			if page.NextCursor != "" {
				t.Errorf("the last page handed back cursor %q — a client walking until the "+
					"cursor is empty would loop", page.NextCursor)
			}
			break
		}
		if page.NextCursor == "" {
			t.Fatal("a page says there is another and hands back nothing to fetch it with")
		}
		cursor = page.NextCursor
	}

	for id := range filed {
		if seen[id] == 0 {
			t.Errorf("proposal %s was on no page of the walk, so nobody can answer it", id)
		}
		if seen[id] > 1 {
			t.Errorf("proposal %s appeared on %d pages — a reviewer decides it twice", id, seen[id])
		}
	}
}

// THE TIE-BREAK, which is the half of the key an id-only cursor loses.
//
// A batch of confirm links goes out together and the answers come back
// together, so two submissions sharing a `submitted_at` is ordinary rather than
// rare. A walk resumed on the instant alone re-reads them; one resumed on the
// id alone skips every row whose instant is later and whose id happens to be
// smaller. The order is by both, so the cursor is by both.
func TestAWalkCrossesProposalsThatArrivedInTheSameInstant(t *testing.T) {
	e := setupChannelConsent(t)
	for i := range 3 {
		proposeCorrection(t, e, ConfirmFieldFullName, fmt.Sprintf("Together %d", i))
	}
	// One instant for all three, so the id is the only thing left to order by.
	// Set on the rows the real door wrote rather than on rows of this test's
	// own: what is under test is the ORDER, and the writer has already run.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact_confirm_submission SET submitted_at = now() WHERE contact_id = $1`,
		e.contact); err != nil {
		t.Fatalf("giving the three one instant: %v", err)
	}

	seen := map[ids.UUID]bool{}
	cursor := ""
	for pages := 0; pages < 5; pages++ {
		batch, page, err := e.store.ListSubmissions(reviewerCtx(e), ListSubmissionsInput{
			Limit: 1, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		for _, sub := range batch {
			if seen[sub.ID] {
				t.Errorf("proposal %s came back twice", sub.ID)
			}
			seen[sub.ID] = true
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Errorf("the walk saw %d of 3 proposals filed in one instant: a cursor that cannot tell "+
			"them apart drops the ones it cannot name", len(seen))
	}
}

// THE WALK CROSSES FROM THE QUEUE INTO THE ARCHIVE, which is the leading half
// of the key and the one an ordinary cursor has no field for.
//
// Unresolved rows sort first. A cursor that carried only the instant and the id
// would, on the first resolved row, compare as "before" every unresolved one
// and hand the reviewer the queue they have just worked.
func TestTheWalkCrossesFromTheQueueIntoWhatIsAlreadyDecided(t *testing.T) {
	e := setupChannelConsent(t)
	decided := proposeCorrection(t, e, ConfirmFieldFullName, "Already answered")
	if _, err := e.store.ResolveSubmission(reviewerCtx(e), decided,
		ResolveSubmissionInput{Resolution: SubmissionRejected}); err != nil {
		t.Fatalf("deciding the first proposal: %v", err)
	}
	waiting := proposeCorrection(t, e, ConfirmFieldTitle, "Still waiting")

	first, page, err := e.store.ListSubmissions(reviewerCtx(e), ListSubmissionsInput{Limit: 1})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 1 || first[0].ID != waiting {
		t.Fatalf("the first page leads with %v, and an unanswered proposal outranks a decided one", first)
	}
	if !page.HasMore {
		t.Fatal("the queue says it is complete with a decided proposal still unlisted")
	}

	second, page, err := e.store.ListSubmissions(reviewerCtx(e),
		ListSubmissionsInput{Limit: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) != 1 || second[0].ID != decided {
		t.Fatalf("the second page carried %v, want the decided proposal — a cursor with no room "+
			"for the resolved flag re-enters at the top of the queue", second)
	}
	if page.HasMore {
		t.Error("the walk says there is more behind the last of two proposals")
	}
}

// A TOKEN THIS QUEUE DID NOT MINT IS REFUSED, never answered with an empty
// page. Telling a reviewer their queue is clear is the one answer this route
// must not guess at — and the shared envelope proves only that a token is one
// of ours, not that it names a position here.
func TestACursorFromSomewhereElseIsRefusedRatherThanAnswered(t *testing.T) {
	e := setupChannelConsent(t)
	proposeCorrection(t, e, ConfirmFieldFullName, "Waiting")

	elsewhere, err := storekit.EncodeOpaque(storekit.Cursor{
		CreatedAt: time.Now(), ID: ids.NewV7(),
	})
	if err != nil {
		t.Fatalf("minting another route's cursor: %v", err)
	}
	for what, token := range map[string]string{
		"another route's cursor": elsewhere,
		"not a token at all":     "not-a-cursor",
	} {
		_, _, err := e.store.ListSubmissions(reviewerCtx(e),
			ListSubmissionsInput{Cursor: token})
		var malformed *storekit.MalformedCursorError
		if !errors.As(err, &malformed) {
			t.Errorf("%s answered %v, want a refusal: an empty page here reads as "+
				"a queue with nothing left in it", what, err)
		}
	}
}

// THE WIRE carries the page, not just the store.
//
// The store can walk correctly and the route still answer a body with no
// continuation in it — the envelope is assembled in the handler, and a client
// walks what the wire says rather than what the store returned.
func TestTheSubmissionQueuesWireCarriesItsPage(t *testing.T) {
	e := setupChannelConsent(t)
	for i := range 3 {
		proposeCorrection(t, e, ConfirmFieldFullName, fmt.Sprintf("Wire %d", i))
	}
	h := Handlers{store: e.store}
	ctx := reviewerCtx(e)

	read := func(cursor *string) ([]crmcontracts.ConfirmSubmission, crmcontracts.PageInfo) {
		t.Helper()
		limit := 2
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/confirm-submissions", nil).WithContext(ctx)
		h.ListConfirmSubmissions(rec, req, crmcontracts.ListConfirmSubmissionsParams{
			Limit: &limit, Cursor: cursor,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Data []crmcontracts.ConfirmSubmission `json:"data"`
			Page crmcontracts.PageInfo            `json:"page"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decoding the queue's answer: %v", err)
		}
		return body.Data, body.Page
	}

	first, page := read(nil)
	if len(first) != 2 || !page.HasMore {
		t.Fatalf("first page carried %d proposals and has_more %v, want 2 and true",
			len(first), page.HasMore)
	}
	if page.NextCursor == nil || *page.NextCursor == "" {
		t.Fatal("the wire says there is another page and hands back no cursor to fetch it with")
	}

	second, page := read(page.NextCursor)
	if len(second) != 1 || page.HasMore {
		t.Fatalf("second page carried %d proposals and has_more %v, want 1 and false",
			len(second), page.HasMore)
	}
	if page.NextCursor != nil {
		t.Errorf("the last page handed back cursor %q — a client walking until the cursor is "+
			"null would loop", *page.NextCursor)
	}
	if second[0].Id == first[0].Id || second[0].Id == first[1].Id {
		t.Error("the second page repeated a proposal from the first")
	}
}
