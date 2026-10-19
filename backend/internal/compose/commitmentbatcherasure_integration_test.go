// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestErasingOneConversationWithdrawsItsTasksAndPendingPromises(t *testing.T) {
	e := integration.Setup(t)
	subject := e.SeedContact(t, "Evidence author", &e.Rep1)
	buyer := ids.From[ids.ContactKind](e.SeedContact(t, "Account buyer", &e.Rep1))
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	quotes := make([]string, 8)
	for i := range quotes {
		quotes[i] = fmt.Sprintf("I will send report %d tomorrow.", i+1)
	}
	body := strings.Join(quotes, " ")
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: &body, OccurredAt: new(now.Add(-time.Hour)), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: subject}},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := ids.UUID(row.Id)
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	var proposals []ids.UUID
	for i, quote := range quotes {
		confidence := 0.75
		want := CommitmentProposed
		if i%3 == 0 {
			confidence, want = 0.95, CommitmentTaskWritten
		}
		c := Commitment{
			Meaning: openCommitmentMeaning("2026-10-19", quote),
			AsOf:    now, Extractor: signalScanActor, SourceActivityID: source, Summary: quote, Quote: quote,
			Body: quote, Seat: &e.Rep1, PromisedTo: &buyer, DueDate: "2026-10-19", Confidence: confidence,
			Locator:  commitmentLocator(source, claimKindOurs, "seat:"+e.Rep1.String(), quote),
			Evidence: approvals.Evidence{SourceType: "activity", SourceID: source, Snippet: quote},
		}
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			outcome, work, err := dispatcher.DispatchTx(e.Admin(), tx, c, ids.NewV7())
			if err == nil && outcome == CommitmentProposed {
				proposals = append(proposals, work)
			}
			if err == nil && outcome != want {
				t.Errorf("promise %d outcome=%s, want %s", i, outcome, want)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1`, source); got != 8 {
		t.Fatalf("admissions=%d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1`, source); got != 8 {
		t.Fatalf("claims=%d, want 8", got)
	}
	if err := privacy.NewEraser(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB())).EraseContact(e.Admin(), subject, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_activity_id=$1 AND kind='task' AND archived_at IS NOT NULL AND body IS NULL AND subject NOT LIKE '%report%'`, source); got != 3 {
		t.Fatalf("erased tasks=%d, want 3", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE id=ANY($1::uuid[]) AND kind='commitment_task' AND status='expired' AND proposed_change='{}'::jsonb AND evidence IS NULL`, proposals); got != 5 {
		t.Fatalf("withdrawn proposals=%d, want 5", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1`, source); got != 0 {
		t.Fatalf("erased conversation retained %d claims", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1`, source); got != 0 {
		t.Fatalf("erased conversation retained %d decisions", got)
	}
}
