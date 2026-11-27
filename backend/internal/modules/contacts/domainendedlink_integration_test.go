// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A link a user removed stays removed. No automatic writer links a contact
// to a company where somebody ended or archived their employment.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asRelationshipEditor is the rep's seat with the right to end or archive an
// employment.
func (e *dedupeEnv) asRelationshipEditor() context.Context {
	ctx := e.as()
	actor, _ := principal.Actor(ctx)
	grant := actor.Permissions.Objects["relationship"]
	grant.Update, grant.Delete = true, true
	actor.Permissions.Objects["relationship"] = grant
	return principal.WithActor(ctx, actor)
}

// employedByTheSweep creates a company on domain, lets the sweep employ the
// contact there and answers the employment's id.
func (e *dedupeEnv) employedByTheSweep(ctx context.Context, t *testing.T, contact ids.ContactID, name, domain string) (ids.CompanyID, ids.UUID) {
	t.Helper()
	company := e.createNamedCompany(ctx, t, name, domain)
	e.sweepOnce(ctx, t, company)
	if got := e.employerOf(ctx, t, contact); got == nil || *got != company.UUID {
		t.Fatalf("the sweep left the contact at %v, want %s: the test proves nothing", got, company)
	}
	var edge ids.UUID
	e.inTx(ctx, t, "reading the employment", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM relationship
			WHERE contact_id = $1 AND company_id = $2 AND kind = 'employment'`, contact, company).Scan(&edge)
	})
	return company, edge
}

// sweepOnce runs the daily reconcile's two steps for one company and answers
// how many contacts it planted.
func (e *dedupeEnv) sweepOnce(ctx context.Context, t *testing.T, company ids.CompanyID) int {
	t.Helper()
	owed, err := e.store.DomainsOwedTheirContacts(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	planted := 0
	for _, one := range owed {
		if one.CompanyID != company {
			continue
		}
		n, err := e.store.AttachDomainBacklog(ctx, one)
		if err != nil {
			t.Fatal(err)
		}
		planted += n
	}
	return planted
}

func (e *dedupeEnv) offeredToTheSweep(ctx context.Context, t *testing.T, company ids.CompanyID) bool {
	t.Helper()
	owed, err := e.store.DomainsOwedTheirContacts(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, one := range owed {
		if one.CompanyID == company {
			return true
		}
	}
	return false
}

func (e *dedupeEnv) liveEmployments(ctx context.Context, t *testing.T, contact ids.ContactID) int {
	t.Helper()
	return e.countInWorkspace(ctx, t, `SELECT count(*) FROM relationship
		WHERE contact_id = $1 AND `+employment.LiveSlotSQL(""), contact)
}

func TestARemovedLinkIsNotMadeAgainByTheSweep(t *testing.T) {
	ended := func(e *dedupeEnv, ctx context.Context, t *testing.T, edge ids.UUID) {
		day := clocktest.Now(t).UTC().AddDate(0, 0, -1)
		if _, err := e.store.UpdateRelationship(e.asRelationshipEditor(), edge, UpdateRelationshipInput{EndedAt: &day}); err != nil {
			t.Fatalf("ending the employment: %v", err)
		}
	}
	archived := func(e *dedupeEnv, ctx context.Context, t *testing.T, edge ids.UUID) {
		if _, err := e.store.ArchiveRelationship(e.asRelationshipEditor(), edge, nil); err != nil {
			t.Fatalf("archiving the employment: %v", err)
		}
	}
	for name, remove := range map[string]func(*dedupeEnv, context.Context, *testing.T, ids.UUID){
		"an ended employment": ended, "an archived employment": archived,
	} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			left := e.openTriageFirst(ctx, t, "left@removed.test", "Left Leaver", "removed.test")
			company, edge := e.employedByTheSweep(ctx, t, left.ContactID, "Removed GmbH", "removed.test")
			remove(e, ctx, t, edge)

			if e.offeredToTheSweep(ctx, t, company) {
				t.Fatal("the sweep offers a company whose only candidate had their job there ended")
			}
			if planted := e.sweepOnce(ctx, t, company); planted != 0 {
				t.Fatalf("the sweep planted %d employments, want 0", planted)
			}
			// Also the write itself, for a caller that skips the selector.
			e.inTx(ctx, t, "planting directly", func(tx pgx.Tx) error {
				n, err := plantDomainEmployment(ctx, tx, "removed.test", company)
				if err == nil && n != 0 {
					t.Errorf("plantDomainEmployment planted %d, want 0", n)
				}
				return err
			})
			if n := e.liveEmployments(ctx, t, left.ContactID); n != 0 {
				t.Fatalf("the contact has %d live employments, want none", n)
			}
		})
	}
}

func TestCaptureDoesNotRelinkAContactWhoseJobWasEnded(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.openTriageFirst(ctx, t, "left@captured.test", "Left Leaver", "captured.test")
	_, edge := e.employedByTheSweep(ctx, t, left.ContactID, "Captured GmbH", "captured.test")
	day := clocktest.Now(t).UTC().AddDate(0, 0, -1)
	if _, err := e.store.UpdateRelationship(e.asRelationshipEditor(), edge, UpdateRelationshipInput{EndedAt: &day}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "left@captured.test", "Left Leaver", "captured.test")); err != nil {
		t.Fatal(err)
	}
	if n := e.liveEmployments(ctx, t, left.ContactID); n != 0 {
		t.Fatalf("a later mail gave the contact %d live employments at the company they left", n)
	}
}

func TestTheSweepStillAttachesAContactNobodyUnlinked(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.openTriageFirst(ctx, t, "left@mixed.test", "Left Leaver", "mixed.test")
	stays := e.openTriage(ctx, t, "stays@mixed.test", "Stays Here", "mixed.test")
	company := e.createNamedCompany(ctx, t, "Mixed GmbH", "mixed.test")
	if planted := e.sweepOnce(ctx, t, company); planted != 2 {
		t.Fatalf("the first sweep planted %d, want both contacts", planted)
	}
	var edge ids.UUID
	e.inTx(ctx, t, "reading the employment", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM relationship WHERE contact_id = $1 AND company_id = $2 AND kind = 'employment'`,
			left.ContactID, company).Scan(&edge)
	})
	day := clocktest.Now(t).UTC().AddDate(0, 0, -1)
	if _, err := e.store.UpdateRelationship(e.asRelationshipEditor(), edge, UpdateRelationshipInput{EndedAt: &day}); err != nil {
		t.Fatal(err)
	}
	if planted := e.sweepOnce(ctx, t, company); planted != 0 {
		t.Fatalf("the sweep planted %d after the leaver's job ended, want 0", planted)
	}
	if got := e.employerOf(ctx, t, stays.ContactID); got == nil || *got != company.UUID {
		t.Fatalf("the contact nobody unlinked works at %v, want %s", got, company)
	}
	if n := e.liveEmployments(ctx, t, left.ContactID); n != 0 {
		t.Fatalf("the leaver has %d live employments, want none", n)
	}
}

func TestAJobEndedElsewhereDoesNotStopTheSweep(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	triaged := e.openTriageFirst(ctx, t, "jo@elsewhere.test", "Jo Elsewhere", "elsewhere.test")
	other := e.createNamedCompany(ctx, t, "Other Employer")
	e.employCurrently(ctx, t, triaged.ContactID, other)
	var edge ids.UUID
	e.inTx(ctx, t, "reading the employment", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM relationship WHERE contact_id = $1 AND company_id = $2 AND kind = 'employment'`,
			triaged.ContactID, other).Scan(&edge)
	})
	day := clocktest.Now(t).UTC().AddDate(0, 0, -1)
	if _, err := e.store.UpdateRelationship(e.asRelationshipEditor(), edge, UpdateRelationshipInput{EndedAt: &day}); err != nil {
		t.Fatal(err)
	}
	company := e.createNamedCompany(ctx, t, "Elsewhere GmbH", "elsewhere.test")
	if planted := e.sweepOnce(ctx, t, company); planted != 1 {
		t.Fatalf("the sweep planted %d, want the contact whose job elsewhere ended", planted)
	}
}

func TestAUserCanLinkAContactAgainAfterTheJobWasEnded(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.openTriageFirst(ctx, t, "left@relink.test", "Left Leaver", "relink.test")
	company, edge := e.employedByTheSweep(ctx, t, left.ContactID, "Relink GmbH", "relink.test")
	day := clocktest.Now(t).UTC().AddDate(0, 0, -1)
	if _, err := e.store.UpdateRelationship(e.asRelationshipEditor(), edge, UpdateRelationshipInput{EndedAt: &day}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.LinkCompanyContacts(ctx, company, []ids.UUID{left.ContactID.UUID}); err != nil {
		t.Fatalf("linking from the offer: %v", err)
	}
	if got := e.employerOf(ctx, t, left.ContactID); got == nil || *got != company.UUID {
		t.Fatalf("the contact works at %v after a user linked them, want %s", got, company)
	}
}

func TestTheConfirmAndItsCountSkipAContactWhoseJobAtTheSuggestedCompanyEnded(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	twin := e.createNamedCompany(ctx, t, "Twin GmbH", "twin-one.test")
	left := e.openTriageFirst(ctx, t, "left@twin-two.test", "Left Leaver", "twin-two.test")
	e.openTriage(ctx, t, "stays@twin-two.test", "Stays Here", "twin-two.test")
	e.employCurrently(ctx, t, left.ContactID, twin)
	var edge ids.UUID
	e.inTx(ctx, t, "reading the employment", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM relationship WHERE contact_id = $1 AND company_id = $2 AND kind = 'employment'`,
			left.ContactID, twin).Scan(&edge)
	})
	day := clocktest.Now(t).UTC().AddDate(0, 0, -1)
	if _, err := e.store.UpdateRelationship(e.asRelationshipEditor(), edge, UpdateRelationshipInput{EndedAt: &day}); err != nil {
		t.Fatal(err)
	}
	e.inTx(ctx, t, "withholding for a name twin", func(tx pgx.Tx) error {
		return withholdForNearDuplicate(ctx, tx, ResolveDomainTriageInput{Domain: "twin-two.test"},
			CompanyCandidateScore{CompanyID: twin, CandidateValue: "Twin GmbH", IncumbentValue: "Twin GmbH"})
	})
	got := e.questionAbout(ctx, t, "twin-two.test").Context
	if got.LinkedContactCount != 1 {
		t.Fatalf("linked contacts = %d, want 1: the contact who left the suggested company is not counted", got.LinkedContactCount)
	}
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "twin-two.test", Status: DomainCompany, Source: DomainSourceHuman, CompanyID: &twin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.EdgesPlanted != got.LinkedContactCount {
		t.Fatalf("the confirm employed %d contacts, the copy promised %d", res.EdgesPlanted, got.LinkedContactCount)
	}
}

func TestAnEnrichmentDoesNotRelinkAContactWhoseJobWasEnded(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	left := e.openTriageFirst(ctx, t, "left@bought.test", "Left Leaver", "bought.test")
	company, edge := e.employedByTheSweep(ctx, t, left.ContactID, "Bought GmbH", "bought.test")
	day := clocktest.Now(t).UTC().AddDate(0, 0, -1)
	if _, err := e.store.UpdateRelationship(e.asRelationshipEditor(), edge, UpdateRelationshipInput{EndedAt: &day}); err != nil {
		t.Fatal(err)
	}
	e.inTx(ctx, t, "planting the bought employment", func(tx pgx.Tx) error {
		_, planted, err := plantProviderEmploymentEdge(ctx, tx, left.ContactID.UUID, company, "databroker", "Head of Sales")
		if err == nil && planted {
			t.Error("the provider's answer re-linked a contact whose job at the company was ended")
		}
		return err
	})
}

func (e *dedupeEnv) applyEvidence(ctx context.Context, t *testing.T, contact ids.ContactID, company ids.CompanyID, evidence employmentEvidence, automatic bool) string {
	t.Helper()
	evidence.claimID, evidence.runID, evidence.provider = ids.NewV7(), ids.NewV7(), "databroker"
	var state string
	e.inTx(ctx, t, "applying the evidence", func(tx pgx.Tx) error {
		_, got, err := e.store.linkEmploymentEpisode(ctx, tx, contact, company, evidence, connectorCapturedBy("databroker"), automatic)
		state = got
		return err
	})
	return state
}

func TestRetainedProviderEvidenceDoesNotRemakeARemovedLink(t *testing.T) {
	ended := func(e *dedupeEnv, t *testing.T, edge ids.UUID) {
		day := clocktest.Now(t).UTC().AddDate(0, 0, -1)
		if _, err := e.store.UpdateRelationship(e.asRelationshipEditor(), edge, UpdateRelationshipInput{EndedAt: &day}); err != nil {
			t.Fatal(err)
		}
	}
	archived := func(e *dedupeEnv, t *testing.T, edge ids.UUID) {
		if _, err := e.store.ArchiveRelationship(e.asRelationshipEditor(), edge, nil); err != nil {
			t.Fatal(err)
		}
	}
	differing := employmentEvidence{Role: "Head of Sales", Started: "2021-03-01", Status: employmentCurrent}
	for name, tc := range map[string]struct {
		remove   func(*dedupeEnv, *testing.T, ids.UUID)
		evidence employmentEvidence
	}{
		"an ended job":                       {ended, employmentEvidence{Role: "Engineer", Status: employmentCurrent}},
		"an archived job":                    {archived, employmentEvidence{Role: "Engineer", Status: employmentCurrent}},
		"a later episode with another role":  {ended, differing},
		"a later episode on an archived job": {archived, differing},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			left := e.openTriageFirst(ctx, t, "left@evidence.test", "Left Leaver", "evidence.test")
			company, edge := e.employedByTheSweep(ctx, t, left.ContactID, "Evidence GmbH", "evidence.test")
			tc.remove(e, t, edge)

			if state := e.applyEvidence(ctx, t, left.ContactID, company, tc.evidence, true); state != employmentNeedsReview {
				t.Fatalf("the automatic apply answered %q, want %q", state, employmentNeedsReview)
			}
			if n := e.liveEmployments(ctx, t, left.ContactID); n != 0 {
				t.Fatalf("the automatic apply left %d live employments, want none", n)
			}
			if state := e.applyEvidence(ctx, t, left.ContactID, company, tc.evidence, false); state != employmentLinked {
				t.Fatalf("a user's resolve answered %q, want %q: a human keeps the override", state, employmentLinked)
			}
		})
	}
}
