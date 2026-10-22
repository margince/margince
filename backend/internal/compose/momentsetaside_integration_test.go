// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The contact page's "Not now" and its undo, over a real database. A moment
// put away gives the card to the next promise. The undo hands it back.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheUndoOfAContactMomentSetAsideShowsItAgain(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "nichtjetzt@kunde.example")
	contactID := ids.From[ids.ContactKind](contact)
	now := clocktest.Now(t)
	clock := func() time.Time { return now }
	logTaskFor(t, e, contact, "Send the signed contract", new(now.Add(24*time.Hour)))
	logTaskFor(t, e, contact, "Send the invoice", new(now.Add(72*time.Hour)))

	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), clock, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), clock)
	headline := func() string {
		t.Helper()
		page, err := svc.Assemble(e.Admin(), contactID)
		if err != nil {
			t.Fatalf("assembling contact360: %v", err)
		}
		if page.Moment == nil {
			t.Fatal("no moment on a contact that owes two promises")
		}
		return page.Moment.Headline
	}

	const first = "You owe them: Send the signed contract"
	before := headline()
	if before != first {
		t.Fatalf("headline = %q, want the soonest promise %q", before, first)
	}
	page, err := svc.Assemble(e.Admin(), contactID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DismissMoment(e.Admin(), contactID, crmcontracts.DismissContactMomentRequest{
		ClaimKey: page.Moment.ClaimKey, EvidenceFingerprint: page.Moment.EvidenceFingerprint,
	}); err != nil {
		t.Fatalf("setting the moment aside: %v", err)
	}
	if got := headline(); got == first {
		t.Fatalf("the set-aside promise still leads the card (%q)", got)
	}
	dismissed := crmcontracts.DismissContactMomentRequest{
		ClaimKey: page.Moment.ClaimKey, EvidenceFingerprint: page.Moment.EvidenceFingerprint,
	}
	if err := svc.RestoreMoment(e.Admin(), contactID, dismissed); err != nil {
		t.Fatalf("undoing the set-aside: %v", err)
	}
	if got := headline(); got != first {
		t.Errorf("after the undo the card leads with %q, want %q back", got, first)
	}
	// Idempotent: undoing what is no longer set aside is the same success.
	if err := svc.RestoreMoment(e.Admin(), contactID, dismissed); err != nil {
		t.Errorf("a second undo: %v, want success", err)
	}
}

// An Undo names the evidence it undoes. The toast from an earlier dismissal
// must not clear a newer one made after the evidence changed.
func TestAnOldUndoLeavesANewerSetAsideStanding(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "zweimal@kunde.example")
	contactID := ids.From[ids.ContactKind](contact)
	now := clocktest.Now(t)
	clock := func() time.Time { return now }
	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), clock, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), clock)
	older := crmcontracts.DismissContactMomentRequest{ClaimKey: "moment:gone_quiet", EvidenceFingerprint: "fp-before"}
	newer := crmcontracts.DismissContactMomentRequest{ClaimKey: "moment:gone_quiet", EvidenceFingerprint: "fp-after"}
	for _, dismissal := range []crmcontracts.DismissContactMomentRequest{older, newer} {
		if err := svc.DismissMoment(e.Admin(), contactID, dismissal); err != nil {
			t.Fatalf("setting the moment aside: %v", err)
		}
	}
	if err := svc.RestoreMoment(e.Admin(), contactID, older); err != nil {
		t.Fatalf("the old toast's undo: %v", err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM contact_moment_dismissal
		WHERE claim_key = 'moment:gone_quiet' AND evidence_fingerprint = 'fp-after'`); count != 1 {
		t.Errorf("the newer set-aside rows = %d after the old undo, want 1 still standing", count)
	}
}
