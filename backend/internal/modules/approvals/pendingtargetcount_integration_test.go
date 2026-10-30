// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// targetCount reads the record count and the record list the company page
// reads.
func (e *stagingEnv) targetCount(as context.Context, t *testing.T, target ids.UUID) (count, listed int) {
	t.Helper()
	if err := e.svc.db.Tx(as, func(tx pgx.Tx) error {
		pending, err := e.svc.PendingForTarget(as, tx, tableCompany, target)
		count, listed = pending.Total, len(pending.Approvals)
		return err
	}); err != nil {
		t.Fatalf("reading the record's pending proposals: %v", err)
	}
	return count, listed
}

// The fixture is past one scan batch of pending, lazily expired and rejected
// proposals on one account, beside proposals on another. The record count must
// equal every page of the inbox narrowed to that account, while the record list
// stays at its cap.
func TestTheRecordCountIsTheInboxCountForThatRecordPastTheScanCap(t *testing.T) {
	e := setupStaging(t)
	target := e.seedCompany(t)
	other := e.seedCompany(t)
	agent := e.asAgent(t)
	decider := seatFor(e.ws, e.rep, tableCompany)
	short := time.Hour
	for i := range 3*PendingScanCap + 17 {
		in := StageInput{
			Kind: kindCompanyNamePromotion, ProposedChange: []byte(`{"proposed_name":"Gitex Global"}`),
			DiffHash: fmt.Sprintf("target-count-%d", i), TargetType: tableCompany, TargetID: target,
			Summary: "Rename Gitex?",
		}
		if i%6 == 3 {
			in.TargetID = other
		}
		if i%4 == 1 {
			in.TTL = &short
		}
		id, err := e.svc.Stage(agent, in)
		if err != nil {
			t.Fatal(err)
		}
		if i%5 == 2 {
			if _, err := e.svc.Decide(decider, id, false, nil); err != nil {
				t.Fatalf("rejecting a fixture: %v", err)
			}
		}
	}
	// Wall time, because staging stamps expires_at from Postgres now().
	later := time.Now().Add(2 * short)
	e.svc.now = func() time.Time { return later }

	company := tableCompany
	want := pagedPending(decider, t, e.svc, ListInput{TargetType: &company, TargetID: &target, IncludeMuted: true})
	count, listed := e.targetCount(decider, t, target)
	if count != len(want) {
		t.Fatalf("record count %d, the inbox narrowed to the record holds %d", count, len(want))
	}
	if count <= PendingScanCap {
		t.Fatalf("the fixture left %d pending, not past the scan cap %d, so a capped count would pass", count, PendingScanCap)
	}
	if listed > PendingScanCap {
		t.Errorf("the record list carried %d rows past its cap %d", listed, PendingScanCap)
	}

	outsider := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Outsider')`,
		outsider, "outsider-"+outsider.String()+"@st.test"); err != nil {
		t.Fatalf("seeding the outsider: %v", err)
	}
	if count, _ := e.targetCount(sharedSeat(e.ws, outsider), t, target); count != 0 {
		t.Errorf("a seat that can decide none of them counted %d", count)
	}
}

// Triage is human work, so a caller acting for nobody is refused the record's
// proposals. An empty list would read as "none waiting".
func TestARecordsPendingProposalsAreRefusedToACallerActingForNobody(t *testing.T) {
	e := setupStaging(t)
	target := e.seedCompany(t)
	as := e.sweeping()
	err := e.svc.db.Tx(as, func(tx pgx.Tx) error {
		_, err := e.svc.PendingForTarget(as, tx, tableCompany, target)
		return err
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a system caller read the record's pending proposals: err=%v, want ErrPermissionDenied", err)
	}
}
