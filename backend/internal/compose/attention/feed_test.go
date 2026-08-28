// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The feed's promise is that a rep can read it and know what their day holds.
// Each test below is one way that promise can be broken.

var readInstant = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return readInstant }

type stubApprovals struct {
	rows []crmcontracts.Approval
	// pending is what the queue HOLDS, which the lane's cap may be well below.
	pending int
	err     error
}

func (s stubApprovals) ListWire(context.Context, ApprovalQuery) ([]crmcontracts.Approval, error) {
	return s.rows, s.err
}

func (s stubApprovals) CountPending(context.Context) (int, error) {
	if s.pending > 0 {
		return s.pending, s.err
	}
	return len(s.rows), s.err
}

type stubDuplicates struct {
	pairs []DuplicatePair
	open  int
	err   error
	// unreadable is a record this caller may not name. The pair still comes
	// back from the queue, which is the case that matters: a visible pair
	// pointing at a record the reader cannot see.
	unreadable ids.UUID
	// describeErr is a read that BROKE rather than one that was refused.
	describeErr error
}

func (s stubDuplicates) OpenCandidates(context.Context, int) ([]DuplicatePair, error) {
	return s.pairs, s.err
}

func (s stubDuplicates) CountOpen(context.Context) (int, error) { return s.open, s.err }

func (s stubDuplicates) Describe(
	_ context.Context, _ string, id ids.UUID,
) (RecordFace, error) {
	if s.describeErr != nil {
		return RecordFace{}, s.describeErr
	}
	if id == s.unreadable {
		return RecordFace{}, apperrors.ErrNotFound
	}
	return RecordFace{Label: "Record " + id.String()[:8]}, nil
}

type stubTasks struct {
	rows []Task
	err  error
	// until records the window the lane asked for, so a test can prove the
	// boundary rather than assume it. A pointer receiver only where it is
	// read back; the value form stays valid for every test that ignores it.
	until *time.Time
}

func (s *stubTasks) OpenForViewer(_ context.Context, until time.Time, _ int) ([]Task, error) {
	s.until = &until
	return s.rows, s.err
}

type stubReceipts struct {
	rows []Receipt
	err  error
}

func (s stubReceipts) Recent(context.Context, time.Time, int) ([]Receipt, error) {
	return s.rows, s.err
}

type stubBriefing struct {
	rows []BriefEntry
	// ran mirrors the seam's own contract: a found run with zero unanswered
	// entries is ran=true, a night that produced nothing is ran=false. The
	// zero value is the no-run morning, so a test must opt in to a run.
	ran bool
	err error
}

func (s stubBriefing) Queue(context.Context) ([]BriefEntry, bool, error) {
	return s.rows, s.ran || len(s.rows) > 0, s.err
}

func approval(summary string) crmcontracts.Approval {
	return crmcontracts.Approval{
		Id:      openapi_types.UUID(ids.NewV7()),
		Kind:    "send_email",
		Summary: &summary,
	}
}

func TestADuplicateOutranksAnApprovalBecauseAMergeCannotBeUndone(t *testing.T) {
	svc := NewService(
		stubApprovals{rows: []crmcontracts.Approval{approval("Send the Weber follow-up")}},
		stubDuplicates{pairs: []DuplicatePair{{ID: ids.NewV7(), EntityType: "person", Confidence: 0.9}}, open: 1},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(out.NeedsYou) != 2 {
		t.Fatalf("needs_you carries %d items, want 2", len(out.NeedsYou))
	}
	if got := out.NeedsYou[0].Source; got != "dedupe_candidate" {
		t.Errorf("the lane leads with %q; a merge has no undo, so it outranks a reversible proposal", got)
	}
}

func TestAWithheldLaneIsNamedRatherThanReportedEmpty(t *testing.T) {
	svc := NewService(
		stubApprovals{},
		stubDuplicates{err: apperrors.ErrPermissionDenied},
		&stubTasks{rows: []Task{{ID: ids.NewV7(), Subject: "Call Anna"}}},
		stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("a refused lane must not fail the read: %v", err)
	}
	if out.LanesOmitted == nil {
		t.Fatal("the refused lane is not named — a rep would read a clear day the server cannot see")
	}
	if got := (*out.LanesOmitted)[0]; got != "needs_you" {
		t.Errorf("omitted lane is %q, want needs_you", got)
	}
	if len(out.NeedsYou) != 0 {
		t.Error("a withheld lane must carry no items")
	}
	if len(out.Planned) != 1 {
		t.Error("one lane being withheld must not cost the reader the others")
	}
}

func TestABrokenLaneFailsTheReadRatherThanReadingAsQuiet(t *testing.T) {
	svc := NewService(
		stubApprovals{err: fmt.Errorf("the database is unreachable")},
		stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	if _, err := svc.Assemble(context.Background()); err == nil {
		t.Fatal("a lane that FAILED was reported as an empty day")
	}
}

func TestTheCountReportsTheTotalThoughTheLaneIsBounded(t *testing.T) {
	pairs := make([]DuplicatePair, needsYouPage+5)
	for i := range pairs {
		pairs[i] = DuplicatePair{ID: ids.NewV7(), EntityType: "person", Confidence: 0.8}
	}
	svc := NewService(
		stubApprovals{},
		stubDuplicates{pairs: pairs, open: 40},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(out.NeedsYou) != needsYouPage {
		t.Errorf("the lane shows %d items, want it bounded at %d", len(out.NeedsYou), needsYouPage)
	}
	if out.Counts.NeedsYou != 40 {
		t.Errorf("the count says %d; a bounded lane must still report the true total", out.Counts.NeedsYou)
	}
}

func TestTheCountCoversStagedProposalsToo(t *testing.T) {
	// The first version summed the true dedupe total with the LENGTH of the
	// approvals page, so twenty pending decisions reported as nine — the lane's
	// own cap, read back as if it were the queue.
	staged := make([]crmcontracts.Approval, needsYouPage)
	for i := range staged {
		staged[i] = approval("Send something")
	}
	svc := NewService(
		stubApprovals{rows: staged, pending: 20},
		stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Counts.NeedsYou != 20 {
		t.Errorf("the count says %d; the lane is bounded but the queue is not", out.Counts.NeedsYou)
	}
}

func TestAnOverdueTaskLeadsThePlannedLane(t *testing.T) {
	yesterday := readInstant.Add(-24 * time.Hour)
	later := readInstant.Add(6 * time.Hour)
	svc := NewService(
		stubApprovals{}, stubDuplicates{},
		&stubTasks{rows: []Task{
			{ID: ids.NewV7(), Subject: "Due later today", DueAt: &later},
			{ID: ids.NewV7(), Subject: "Was due yesterday", DueAt: &yesterday},
		}},
		stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if *out.Planned[0].Title != "Was due yesterday" {
		t.Errorf("planned leads with %q; a promise already broken outranks one merely due", *out.Planned[0].Title)
	}
	if out.Planned[0].Overdue == nil || !*out.Planned[0].Overdue {
		t.Error("overdue is resolved server-side so every surface agrees where today ends")
	}
	if out.Planned[1].Overdue == nil || *out.Planned[1].Overdue {
		t.Error("a task due later today is not overdue")
	}
}

// A task names the record it was raised for, so the row can reach it.
//
// The lane's own title is generic by design — an automation writes "Follow up
// with the new lead" for every lead it routes — so without the subject the
// reader is told to follow something up and not told which one. The link is
// what makes the row work rather than merely readable.
func TestATaskCarriesTheRecordItWasRaisedFor(t *testing.T) {
	due := readInstant.Add(-time.Hour)
	lead := ids.NewV7()
	svc := NewService(
		stubApprovals{}, stubDuplicates{},
		&stubTasks{rows: []Task{{
			ID: ids.NewV7(), Subject: "Follow up with the new lead", DueAt: &due,
			LinkType: "lead", LinkID: lead,
		}}},
		stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	subject := out.Planned[0].Subject
	if subject == nil {
		t.Fatal("a task filed under a lead reaches the reader with nowhere to go")
	}
	if subject.Type != "lead" || ids.UUID(subject.Id) != lead {
		t.Errorf("the row points at %s %v, not the lead it was raised for",
			subject.Type, subject.Id)
	}
}

// A task filed under nothing this surface routes to carries no subject.
//
// A hand-written task belongs to whoever wrote it and often names no record at
// all. Inventing one would send the reader somewhere arbitrary, so the row
// stays a row: readable, completable, and pointing nowhere.
func TestATaskUnderNoRecordCarriesNoSubject(t *testing.T) {
	due := readInstant.Add(-time.Hour)
	svc := NewService(
		stubApprovals{}, stubDuplicates{},
		&stubTasks{rows: []Task{{
			ID: ids.NewV7(), Subject: "Call the accountant", DueAt: &due,
		}}},
		stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Planned[0].Subject != nil {
		t.Errorf("an unfiled task claims to be about %v", out.Planned[0].Subject)
	}
}

func TestAReceiptOffersNoDecision(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{},
		stubReceipts{rows: []Receipt{{
			ID: ids.NewV7(), Kind: "close_date_correction",
			Summary: "Moved the Acme close date to 27 Sep", OccurredAt: readInstant.Add(-time.Hour),
		}}},
		stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(out.DoneForYou) != 1 {
		t.Fatalf("done_for_you carries %d items, want 1", len(out.DoneForYou))
	}
	for _, action := range out.DoneForYou[0].Actions {
		if action == "decide" || action == "merge" {
			t.Errorf("a finished act offers %q — it would ask a reader to answer a settled question", action)
		}
	}
}

func TestADuplicateCarriesNoServerWrittenSentence(t *testing.T) {
	svc := NewService(
		stubApprovals{},
		stubDuplicates{pairs: []DuplicatePair{{ID: ids.NewV7(), EntityType: "organization", Confidence: 0.92}}, open: 1},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	item := out.NeedsYou[0]
	// The product ships three languages. A sentence composed in Go reaches a
	// German reader in English, so the client is handed the facts and writes
	// the line itself.
	if item.Title != nil {
		t.Errorf("the server wrote %q for a duplicate; that sentence has no translation", *item.Title)
	}
	if item.Kind == nil || *item.Kind != "organization" {
		t.Error("the client needs the record type to write the line")
	}
	if item.Confidence == nil {
		t.Error("the client needs the confidence to qualify the claim")
	}
}

func TestAFloodOfDuplicatesDoesNotBuryTheStagedDecisions(t *testing.T) {
	// The lane read duplicates to depth, then approvals, then truncated. With
	// more open pairs than the page holds, every slot went to duplicates and a
	// staged decision could not be reached from the one surface that exists to
	// reach it — while the count beside the lane went on reporting all of them.
	pairs := make([]DuplicatePair, needsYouPage+1)
	for i := range pairs {
		pairs[i] = DuplicatePair{ID: ids.NewV7(), EntityType: "organization", Confidence: 0.9}
	}
	staged := make([]crmcontracts.Approval, needsYouPage)
	for i := range staged {
		staged[i] = approval("Send the follow-up")
	}
	svc := NewService(
		stubApprovals{rows: staged, pending: 79},
		stubDuplicates{pairs: pairs, open: len(pairs)},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	var approvals int
	for _, item := range out.NeedsYou {
		if item.Source == "approval" {
			approvals++
		}
	}
	if approvals == 0 {
		t.Fatalf("the page is %d duplicates and no decisions; one producer must not take the whole lane",
			len(out.NeedsYou))
	}
}

func TestADuplicateCardNamesBothRecords(t *testing.T) {
	// Without the pair a card can only say "two companies look like the same
	// one" and send the reader elsewhere to find out which two.
	left, right := ids.NewV7(), ids.NewV7()
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "organization", Confidence: 0.93,
			LeftID: left, RightID: right,
			Evidence: []FieldComparison{{Field: "display_name", Signal: "collide"}},
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	pair := out.NeedsYou[0].Pair
	if pair == nil {
		t.Fatal("the duplicate carries no pair, so the decision cannot be made where it is shown")
	}
	if pair.Left.Label == "" || pair.Right.Label == "" {
		t.Error("a side with no name asks the reader to merge two records they cannot tell apart")
	}
	if len(pair.Evidence) != 1 {
		t.Fatalf("evidence carries %d rows, want the one the detector recorded", len(pair.Evidence))
	}
}

func TestAnUnreadableSideCostsTheMergeVerbRatherThanLeakingTheRecord(t *testing.T) {
	hidden := ids.NewV7()
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, unreadable: hidden, pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "person", Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: hidden,
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("a record this reader may not see must not fail the whole day: %v", err)
	}
	if out.NeedsYou[0].Pair != nil {
		t.Fatal("a side the reader may not read was named anyway")
	}
	for _, action := range out.NeedsYou[0].Actions {
		if action == "merge" {
			t.Error("merge is offered over a record the reader cannot see")
		}
	}
}

func TestEvidenceNeverReachesAReaderAsAColumnName(t *testing.T) {
	// The queue stores whatever the detector wrote. A key the client has no
	// word for would print as itself — which is how `full_name` and `org`
	// reached the screen from the queue this lane replaces.
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "person", Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: ids.NewV7(),
			Evidence: []FieldComparison{
				{Field: "full_name", Signal: "collide"},
				{Field: "some_internal_column", Signal: "collide"},
				{Field: "email", Signal: "unheard_of_verdict"},
				{Field: "org", Signal: "collide"},
			},
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	rows := out.NeedsYou[0].Pair.Evidence
	if len(rows) != 1 {
		t.Fatalf("evidence carries %d rows, want only the one the client can name", len(rows))
	}
	if rows[0].Field != "full_name" {
		t.Errorf("kept %q; the nameable row is the one that survives", rows[0].Field)
	}
}

func TestARecordReadThatBrokeIsNotReportedAsWithheld(t *testing.T) {
	// The refusal path and the failure path arrive at the same call. Collapsing
	// them tells a reader a pair is hidden from their account when the truth is
	// that the database would not answer — a reassuring lie, and the one this
	// surface must never tell.
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, describeErr: fmt.Errorf("the database is unreachable"), pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "person", Confidence: 0.9,
			LeftID: ids.NewV7(), RightID: ids.NewV7(),
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	if _, err := svc.Assemble(context.Background()); err == nil {
		t.Fatal("a record read that FAILED was rendered as a pair the reader may not see")
	}
}

func TestAnIdentityConflictKeepsTheOneRowThatExplainsIt(t *testing.T) {
	// This pair writes exactly ONE evidence row, and an earlier version dropped
	// it: `matched_lane` was recognised and `exact_conflict` was not, so the
	// card named two records and said nothing about why they collided. The
	// field vocabulary alone could not catch it — a row dies on either half.
	lane := "email:anna@example.com"
	svc := NewService(
		stubApprovals{},
		stubDuplicates{open: 1, pairs: []DuplicatePair{{
			ID: ids.NewV7(), EntityType: "person", Confidence: 1,
			LeftID: ids.NewV7(), RightID: ids.NewV7(),
			Evidence: []FieldComparison{
				{Field: "matched_lane", Signal: "exact_conflict", Left: &lane},
			},
		}}},
		&stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	rows := out.NeedsYou[0].Pair.Evidence
	if len(rows) != 1 {
		t.Fatalf("evidence carries %d rows; the pair's only explanation was dropped", len(rows))
	}
	if rows[0].Signal != "exact_conflict" {
		t.Errorf("signal is %q, want the verdict the detector recorded", rows[0].Signal)
	}
}

func TestTheBriefingLaneIsItsOwnAndNotADecision(t *testing.T) {
	deal := ids.NewV7()
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{rows: []BriefEntry{{ID: ids.NewV7(), DealID: deal, Rank: 1}}}, nil, nil, nil,
		nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(out.ThisMorning) != 1 {
		t.Fatalf("this_morning carries %d items, want 1", len(out.ThisMorning))
	}
	// The partition rule, and it is not cosmetic: the focus lane fetches every
	// non-dedupe needs_you item as an approval, so a brief item smuggled in
	// there renders a failed card rather than a brief.
	if len(out.NeedsYou) != 0 {
		t.Fatalf("needs_you carries %d items, want none — a briefing item is a suggestion, not a decision", len(out.NeedsYou))
	}
	if out.Counts.ThisMorning != 1 {
		t.Errorf("this_morning count = %d, want 1", out.Counts.ThisMorning)
	}
	item := out.ThisMorning[0]
	if item.Source != "brief_item" {
		t.Errorf("source = %q, want brief_item — it is how the client knows which endpoint answers it", item.Source)
	}
	if item.Subject == nil || ids.UUID(item.Subject.Id) != deal {
		t.Errorf("subject = %+v, want the deal the item is about", item.Subject)
	}
	if item.Title != nil {
		t.Errorf("title = %q; the sentence is the client's to write, in the reader's own language", *item.Title)
	}
}

func TestAMorningWithNoRunIsEmptyRatherThanWithheld(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.ThisMorning == nil {
		t.Fatal("this_morning is null; the contract promises a list, and a generated client iterates it")
	}
	if len(out.ThisMorning) != 0 {
		t.Fatalf("this_morning carries %d items, want none", len(out.ThisMorning))
	}
	// "The night found nothing" is not "this was hidden from you". Naming it
	// omitted would tell a rep something was withheld when nothing was.
	if out.LanesOmitted != nil {
		t.Errorf("lanes_omitted = %v, want none for a morning that simply has no run", *out.LanesOmitted)
	}
}

func TestABriefingLaneRefusedIsNamedRatherThanReportedQuiet(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{err: apperrors.ErrPermissionDenied}, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.LanesOmitted == nil {
		t.Fatal("a refused briefing lane was reported as an empty morning")
	}
	if got := *out.LanesOmitted; len(got) != 1 || got[0] != "this_morning" {
		t.Fatalf("lanes_omitted = %v, want [this_morning]", got)
	}
}

func TestABrokenBriefingLaneFailsTheReadRatherThanReadingAsQuiet(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{err: errors.New("the brief read fell over")}, nil, nil, nil, nil, nil, fixedClock)
	if _, err := svc.Assemble(context.Background()); err == nil {
		t.Fatal("a broken briefing read was reported as a quiet morning")
	}
}

func TestABriefingItemOffersItsOwnThreeVerbs(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{rows: []BriefEntry{{ID: ids.NewV7(), DealID: ids.NewV7(), Rank: 1}}}, nil, nil, nil,
		nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	got := out.ThisMorning[0].Actions
	want := []crmcontracts.AttentionItemActions{"act", "set_aside", "dismiss"}
	if len(got) != len(want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("actions = %v, want %v", got, want)
		}
	}
	// set_aside, not snooze: a task's snooze moves a due date the rep agreed
	// to, and this hides a suggestion. One word for both would send a client
	// that handles snooze generically to the wrong endpoint.
	for _, action := range got {
		if action == "snooze" {
			t.Error("a briefing item offers snooze, which is the task verb and a different write")
		}
	}
}

// Each lane is read ONCE per feed. A lane assembled twice costs a second
// database round trip on every load of the page, and a refusal from the second
// read names the withheld lane twice — which is how the duplicate announces
// itself to a reader rather than to a test.
//
// It is asserted per lane rather than in one place because the lanes are wired
// separately: adding a fourth by copying a third is exactly the slip that
// leaves an old block behind.
func TestEveryLaneIsReadOncePerFeed(t *testing.T) {
	commitments := &stubCommitments{rows: []Commitment{promise("a promise", readInstant)}}
	meetings := &stubMeetings{rows: []Meeting{{Subject: "a meeting", StartsAt: readInstant}}}
	decay := &stubDecay{rows: []QuietRelationship{{Name: "a contact", QuietDays: 63, LastAt: readInstant}}}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		commitments, stubAtRisk{}, decay, meetings, nil, fixedClock,
	)
	if _, err := svc.Assemble(context.Background()); err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if commitments.calls != 1 {
		t.Errorf("the commitments lane was read %d times, want once", commitments.calls)
	}
	if meetings.calls != 1 {
		t.Errorf("the meetings lane was read %d times, want once", meetings.calls)
	}
	if decay.calls != 1 {
		t.Errorf("the relationship decay lane was read %d times, want once", decay.calls)
	}
}

// A withheld lane appears in lanes_omitted exactly one time. Two entries is
// what a duplicate read looks like on the wire, and a client rendering the list
// would say it twice.
func TestAWithheldLaneAppearsInLanesOmittedExactlyOneTime(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{err: apperrors.ErrPermissionDenied}, stubAtRisk{}, nil, nil, nil, fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	var named int
	for _, lane := range *out.LanesOmitted {
		if lane == "commitments" {
			named++
		}
	}
	if named != 1 {
		t.Errorf("commitments named %d times in lanes_omitted, want once", named)
	}
}
