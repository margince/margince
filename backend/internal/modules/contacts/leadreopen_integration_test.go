// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Reopening a disqualified lead against Postgres: the archive, the reason and
// the note go, and the lead comes back at the status it held when somebody
// closed it — read from the disqualify's own audit row rather than re-derived.
//
// The status is the assertion that matters. Recomputing it from today's
// activity would answer about the lead as it is NOW, and a lead disqualified at
// `contacted` and since emailed twice would come back `engaged` — a claim
// nobody made about a lead nobody was working.

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestReopeningALeadRestoresTheStatusItWasClosedAt(t *testing.T) {
	e := setupPromoteConsent(t)
	now := time.Now().UTC()
	lead := e.seedLeadCreatedAt(t, "reopen-contacted@example.test", now.Add(-time.Hour))

	if _, err := e.store.AdvanceLeadStatus(e.ctx, lead, LeadStatusContacted, now); err != nil {
		t.Fatalf("climb to contacted: %v", err)
	}
	note := "budget gone for this quarter"
	closed, err := e.store.DisqualifyLead(e.ctx, lead, DisqualifyLeadInput{Note: &note})
	if err != nil {
		t.Fatalf("disqualify: %v", err)
	}
	if closed.Status != crmcontracts.LeadStatusDisqualified || closed.ArchivedAt == nil {
		t.Fatalf("after disqualify: status=%s archived=%v, want disqualified and archived", closed.Status, closed.ArchivedAt)
	}

	reopened, err := e.store.ReopenLead(e.ctx, lead)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	// Contacted, not engaged: the lead comes back where it was left, and the
	// fallback is only for a row whose trail no longer says.
	if reopened.Status != crmcontracts.LeadStatusContacted {
		t.Errorf("reopened status = %s, want contacted — the status it was closed at, read from the "+
			"disqualify's audit row", reopened.Status)
	}
	if reopened.ArchivedAt != nil {
		t.Errorf("archived_at = %v, want cleared — a reopened lead is back on the open ladder", reopened.ArchivedAt)
	}
	if reopened.DisqualifyReasonId != nil || reopened.DisqualifyNote != nil {
		t.Errorf("the closure's reason and note survived the reopen: reason=%v note=%v",
			reopened.DisqualifyReasonId, reopened.DisqualifyNote)
	}
	// The lead is readable through the live-only door again, which is what
	// "back on the ladder" means to every list and filter that reads it.
	live, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("read the reopened lead live: %v", err)
	}
	if live.Status != crmcontracts.LeadStatusContacted {
		t.Errorf("live read status = %s, want contacted", live.Status)
	}

	// Reopening again refuses. Succeeding would tell a caller their undo did
	// something, and would restore `disqualified` as this lead's "last status".
	if _, err := e.store.ReopenLead(e.ctx, lead); err == nil {
		t.Error("reopening an open lead succeeded, want a conflict")
	} else {
		var notDisqualified *NotDisqualifiedError
		if !errors.As(err, &notDisqualified) {
			t.Errorf("second reopen: err = %v, want NotDisqualifiedError", err)
		}
	}
}

// A lead whose trail no longer names a status comes back as `engaged` rather
// than refusing: the audit log is retained on its own schedule, and a lead that
// cannot be reopened because of a record it does not own is a dead row.
func TestALeadWhoseClosureIsNoLongerOnTheTrailReopensAsEngaged(t *testing.T) {
	e := setupPromoteConsent(t)
	now := time.Now().UTC()
	lead := e.seedLeadCreatedAt(t, "reopen-untraced@example.test", now.Add(-time.Hour))

	// Closed WITHOUT going through DisqualifyLead, so no audit row names the
	// status. That is the state a lead reaches two ways in production: a row
	// disqualified before the trail recorded the before-image, and one whose
	// audit rows have aged out under their own retention. It cannot be
	// simulated by deleting the row — audit_log refuses a DELETE, which is the
	// append-only rule doing its job.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE lead SET status = 'disqualified', archived_at = now() WHERE id = $1`,
		lead.UUID); err != nil {
		t.Fatalf("closing the lead without a trail: %v", err)
	}

	reopened, err := e.store.ReopenLead(e.ctx, lead)
	if err != nil {
		t.Fatalf("reopen without a trail: %v — a lead nothing can reopen is a dead row", err)
	}
	if reopened.Status != crmcontracts.LeadStatusEngaged {
		t.Errorf("reopened status = %s, want engaged — what a lead somebody is choosing to pursue "+
			"again is; `new` would claim nobody had touched it", reopened.Status)
	}
	if reopened.ArchivedAt != nil {
		t.Error("archived_at survived the reopen")
	}
}
