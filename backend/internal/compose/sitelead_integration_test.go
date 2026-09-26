// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The deep read's contact lane while it is shut (siteLeadCaptureOpen): somebody
// a company's website names becomes nobody's lead until accepting one also sends
// them the Article 14 notice. A read proposes no one, and a proposal staged
// before the lane shut cannot be accepted, one decision at a time or in a bundle.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// acmeTeamSite is a two-page site whose /team page names two contacts: Anna
// with a printed email, Bernd without one: only Anna is proposed, because a
// lead nobody can contact is not a lead.
func acmeTeamSite() *fakeSite {
	return &fakeSite{pages: map[string]fakeSitePage{
		seedURL: {text: readable("Acme home.")},
		seedURL + "/team": {text: readable("Team.") + " Anna Muster is our Chief Executive Officer. " +
			"Reach her at anna@acme.example. Bernd Beispiel leads sales as Head of Sales."},
	}}
}

// teamDeepBrain names both contacts on the team page; Bernd's claimed
// email is NOT printed on the page, so the gate must strip it while
// keeping him. The profile lane grounds nothing.
func teamDeepBrain() laneFake {
	return laneFake{
		profileReply: `{"fields":[]}`,
		pageReplies: map[string]string{
			seedURL + "/team": `{"facts":[],"contacts":[
				{"n":"Anna Muster","r":"Chief Executive Officer","q":"Anna Muster is our Chief Executive Officer","m":"anna@acme.example","e":"s0"},
				{"n":"Bernd Beispiel","r":"Head of Sales","q":"Bernd Beispiel leads sales as Head of Sales","m":"bernd@acme.example","e":"s0"}]}`,
		},
	}
}

// runTeamDeepRead crawls acmeTeamSite with the contacts reply as the one
// corpus answer and returns the finished dossier.
func runTeamDeepRead(t *testing.T, e *integration.Env, company ids.UUID) (contacts.SiteRead, *approvals.Service) {
	t.Helper()
	return runTeamDeepReadOn(t, e, company, acmeTeamSite(), teamDeepBrain())
}

// runTeamDeepReadOn is runTeamDeepRead over a caller-chosen site and corpus
// answer, for the reads that need the page to say something different.
func runTeamDeepReadOn(t *testing.T, e *integration.Env, company ids.UUID, site *fakeSite, brain laneFake) (contacts.SiteRead, *approvals.Service) {
	t.Helper()
	worker, svc := newDeepReadTestWorker(e, site, brain)
	read, args := startDeepRead(t, e, company)
	if err := worker.run(context.Background(), args); err != nil {
		t.Fatalf("run: %v", err)
	}
	done, err := e.Contacts.GetSiteRead(e.As(e.Rep1, nil, integration.AdminPerms), companyIDOf(company), read.ID)
	if err != nil {
		t.Fatal(err)
	}
	return done, svc
}

// stageSiteLeadAsBefore stages one site_lead proposal through the lane's own
// writer, the way a read did before the lane shut, and answers its id.
func stageSiteLeadAsBefore(t *testing.T, e *integration.Env, company ids.UUID, bundleID ids.UUID) ids.ApprovalID {
	t.Helper()
	svc := approvalsServiceWithEffects(e.Pool)
	contact := siteContact{
		Name: "Anna Muster", Role: "Chief Executive Officer", PublishedEmail: "anna@acme.example",
		EvidenceSnippet: "Anna Muster is our Chief Executive Officer", SourceURL: seedURL + "/team",
	}
	var id ids.ApprovalID
	readID := ids.NewV7()
	ctx := withClaimedRequester(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), readID)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		in, err := siteLeadStageInput(approvalSummaryCopyIn(ctx, tx), readID, company, seedURL, contact, bundleID)
		if err != nil {
			return err
		}
		id, err = svc.StageOrJoinPendingInTx(ctx, tx, in)
		return err
	}); err != nil {
		t.Fatalf("staging a site lead as a read did before the lane shut: %v", err)
	}
	return id
}

func TestATeamPageReadProposesNoOneWhileTheNoticePathIsMissing(t *testing.T) {
	e := integration.Setup(t)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	done, _ := runTeamDeepRead(t, e, company)

	// The read itself still finishes: the lane is shut, the read is not broken.
	if done.Status != "done" {
		t.Fatalf("dossier status = %q, want done", done.Status)
	}
	if len(done.ProposalIDs) != 0 {
		t.Fatalf("proposal_ids = %v, want none while no notice path exists", done.ProposalIDs)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'site_lead'`); n != 0 {
		t.Fatalf("%d site_lead proposals staged, want 0: nothing is parked for a decision nobody may make", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead`); n != 0 {
		t.Fatalf("%d leads after the read, want 0", n)
	}
}

func TestASiteLeadStagedBeforeTheLaneShutStaysPendingWhenAccepted(t *testing.T) {
	e := integration.Setup(t)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	id := stageSiteLeadAsBefore(t, e, company, ids.NewV7())

	svc := approvalsServiceWithEffects(e.Pool)
	_, err := svc.Decide(e.As(e.Rep2, nil, integration.AdminPerms), id, true, nil)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("accepting a site lead while the lane is shut = %v, want a conflict", err)
	}
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT status FROM approval WHERE id = $1`, id).Scan(&status)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("the refused proposal is %s, want pending: refused before the decision commits", status)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead`); n != 0 {
		t.Fatalf("%d leads after a refused accept, want 0", n)
	}
	// Declining is still open: a human can clear the question.
	if _, err := svc.Decide(e.As(e.Rep2, nil, integration.AdminPerms), id, false, nil); err != nil {
		t.Fatalf("declining the proposal: %v", err)
	}
}

// A bundle approval runs each member's precheck too, so a site lead in it is
// refused before anything is decided and stays pending, where it can still be
// declined. Left approved, it could never be redeemed once its window passed.
func TestABundleApprovalLeavesASiteLeadPendingWhileTheLaneIsShut(t *testing.T) {
	e := integration.Setup(t)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	bundleID := ids.NewV7()
	id := stageSiteLeadAsBefore(t, e, company, bundleID)

	svc := approvalsServiceWithEffects(e.Pool)
	members, err := svc.DecideBundle(e.As(e.Rep2, nil, integration.AdminPerms), bundleID, true, nil)
	if err != nil {
		t.Fatalf("deciding the bundle: %v", err)
	}
	if len(members) != 1 || members[0].Outcome != approvals.BundleRefused {
		t.Fatalf("bundle members = %+v, want the site lead refused", members)
	}
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT status FROM approval WHERE id = $1`, id).Scan(&status)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("the refused member is %s, want pending", status)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead`); n != 0 {
		t.Fatalf("%d leads after approving a bundle holding a site lead, want 0", n)
	}
	if _, err := svc.DecideBundle(e.As(e.Rep2, nil, integration.AdminPerms), bundleID, false, nil); err != nil {
		t.Fatalf("declining the bundle after the refusal: %v", err)
	}
}
