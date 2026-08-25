// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
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
}

func (s stubTasks) OpenForViewer(context.Context, time.Time, int) ([]Task, error) {
	return s.rows, s.err
}

type stubReceipts struct {
	rows []Receipt
	err  error
}

func (s stubReceipts) Recent(context.Context, time.Time, int) ([]Receipt, error) {
	return s.rows, s.err
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
		stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubTasks{rows: []Task{{ID: ids.NewV7(), Subject: "Call Anna"}}},
		stubReceipts{}, fixedClock,
	)
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
		stubDuplicates{}, stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubDuplicates{}, stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubTasks{rows: []Task{
			{ID: ids.NewV7(), Subject: "Due later today", DueAt: &later},
			{ID: ids.NewV7(), Subject: "Was due yesterday", DueAt: &yesterday},
		}},
		stubReceipts{}, fixedClock,
	)
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

func TestAReceiptOffersNoDecision(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, stubTasks{},
		stubReceipts{rows: []Receipt{{
			ID: ids.NewV7(), Kind: "close_date_correction",
			Summary: "Moved the Acme close date to 27 Sep", OccurredAt: readInstant.Add(-time.Hour),
		}}},
		fixedClock,
	)
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
		stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubTasks{}, stubReceipts{}, fixedClock,
	)
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
		stubTasks{}, stubReceipts{}, fixedClock,
	)
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
