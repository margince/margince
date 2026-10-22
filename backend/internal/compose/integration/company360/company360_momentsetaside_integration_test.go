// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The account card's "Not now" and its undo, over a real database.
//
// The card's set-aside lands in the store the advice rows use, keyed on the
// card's evidence fingerprint. So putting the card away must hand it to the
// next promise for this reader only, and the undo must hand it back.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSettingTheAccountMomentAsideHandsTheCardToTheNextPromise(t *testing.T) {
	e := integration.Setup(t)
	svc := company360Service(e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Acme", &e.Rep1))
	logCompanyTask(t, e, company.UUID, "Send the contract", company360Clock.Add(24*time.Hour))
	logCompanyTask(t, e, company.UUID, "Send the invoice", company360Clock.Add(72*time.Hour))
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	const first, second = "You owe them: Send the contract", "You owe them: Send the invoice"
	before, err := svc.Assemble(rep, company)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if before.Moment == nil || before.Moment.Headline != first {
		t.Fatalf("moment = %+v, want the soonest promise %q", before.Moment, first)
	}
	fingerprint := before.Moment.EvidenceFingerprint
	if err := svc.DismissSuggestion(rep, company, fingerprint, nil); err != nil {
		t.Fatalf("setting the moment aside: %v", err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM suggestion_dismissal`); count != 1 {
		t.Fatalf("suggestion_dismissal rows = %d, want the one set-aside stored", count)
	}
	after, err := svc.Assemble(rep, company)
	if err != nil {
		t.Fatalf("re-assemble: %v", err)
	}
	if after.Moment == nil || after.Moment.Headline != second {
		t.Fatalf("moment after the set-aside = %+v, want the next promise %q", after.Moment, second)
	}

	// Another reader's card is theirs: one rep's set-aside hides nothing from them.
	other, err := svc.Assemble(e.Admin(), company)
	if err != nil {
		t.Fatalf("assemble for another reader: %v", err)
	}
	if other.Moment == nil || other.Moment.Headline != first {
		t.Errorf("another reader's moment = %+v, want %q untouched", other.Moment, first)
	}

	if err := svc.RestoreSuggestion(rep, company, fingerprint); err != nil {
		t.Fatalf("undoing the set-aside: %v", err)
	}
	restored, err := svc.Assemble(rep, company)
	if err != nil {
		t.Fatalf("assemble after the undo: %v", err)
	}
	if restored.Moment == nil || restored.Moment.Headline != first {
		t.Errorf("moment after the undo = %+v, want %q back", restored.Moment, first)
	}
}

// logCompanyTask files one open task against the account through the real
// activity writer.
func logCompanyTask(t *testing.T, e *integration.Env, company ids.UUID, subject string, due time.Time) {
	t.Helper()
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}},
	}); err != nil {
		t.Fatalf("logging task %q: %v", subject, err)
	}
}
