// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// What a commitment may NAME, and what happens to that name when the person it
// names exercises erasure.
//
// A file of its own beside weeklyplan_integration_test.go, because these ask a
// different question of the same table: not "whose plan is this" but "whose
// data is on it". A commitment is the one place in this module where a rep's
// own record holds a THIRD PARTY's — the contact a promise is about — and the
// two obligations that follow (refuse a link the rep cannot open, erase the
// name when the subject asks) are invisible to every test next door.

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// planRepPerms is a rep who may plan their week and is BOUND to their own rows.
//
// Its own fixture rather than integration.AdminPerms, which the tests next door
// use, and the difference is the whole point of the two link tests below:
// AdminPerms carries RowScopeAll, and an unbounded actor short-circuits every
// clause auth.EnsureVisibleLive renders — so a refusal test written against it
// cannot fail, whatever the guard does. Rather than widening
// integration.RepPerms, which several suites read as "a rep who cannot see an
// organization", this mirrors the seeded rep row for the one object under test.
var planRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"person":      {Create: true, Read: true, Update: true},
		"weekly_plan": {Create: true, Read: true, Update: true},
		// A person read resolves the basis it reports money in, so every
		// seeded role holds this one; integration.RepPerms carries it for the
		// same reason.
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeOwn,
}

// boundedRep is the caller the link tests use: the same seat as rep1, held to
// its own rows.
func boundedRep(e *planEnv) context.Context {
	return e.As(e.Rep1, []ids.UUID{e.Team1}, planRepPerms)
}

// Art. 17 reaches a commitment that names the subject.
//
// The row survives and stops naming them, which is the ruling this cascade
// makes: the commitment is the REP's record of having worked, and the erasure
// request is the CONTACT's. What must not survive is the label a rep typed
// about a person, the help they asked for, the answer their lead gave, and the
// link that says which person it was.
func TestErasureRedactsACommitmentNamingTheSubject(t *testing.T) {
	e := setupPlan(t)
	subject := e.SeedPerson(t, "Anna Weber", &e.Rep1)

	commitment, err := e.store.AddCommitment(e.rep1Ctx, planClock, weeklyplan.NewCommitment{
		Label:            "Chase Anna Weber about the renewal",
		LinkedRecordType: "person",
		LinkedRecordID:   subject,
	})
	if err != nil {
		t.Fatalf("writing the commitment: %v", err)
	}
	if err := e.store.AskForHelp(e.rep1Ctx, commitment.ID, "Anna Weber will not return my calls"); err != nil {
		t.Fatal(err)
	}
	if err := e.store.Respond(e.rep2Ctx, commitment.ID, "I know Anna Weber, I will introduce you"); err != nil {
		t.Fatal(err)
	}

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), subject, "subject request"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	plan, err := e.store.Current(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commitments) != 1 {
		t.Fatalf("the rep's week came back with %d commitments, wanted the one they wrote — "+
			"an erasure that DELETED the row would discharge the contact's request by "+
			"destroying the employee's work record", len(plan.Commitments))
	}
	got := plan.Commitments[0]

	// Each of the four is checked on its own. A single "does the row still
	// mention Weber" assertion passes as soon as any one of them is cleared,
	// and three plaintext copies of the name would survive it.
	if got.Label == "Chase Anna Weber about the renewal" {
		t.Errorf("the label still reads %q — the rep's own words about the subject survived", got.Label)
	}
	if got.HelpRequested != "" {
		t.Errorf("the help request still reads %q", got.HelpRequested)
	}
	if got.ManagerResponse != "" {
		t.Errorf("the lead's answer still reads %q", got.ManagerResponse)
	}
	if got.LinkedRecordType != "" || !got.LinkedRecordID.IsZero() {
		t.Errorf("the link still points at %s %s — the row still says WHICH person it was, "+
			"which re-identifies a label the erasure just cleared",
			got.LinkedRecordType, got.LinkedRecordID)
	}
}

// The other direction, and without it the test above passes on an eraser that
// blanks every commitment in the installation.
//
// A rep's plan is full of promises about deals and people who never asked for
// anything. One subject's erasure must leave every one of them untouched.
func TestErasureLeavesTheCommitmentsThatNameSomebodyElse(t *testing.T) {
	e := setupPlan(t)
	subject := e.SeedPerson(t, "Anna Weber", &e.Rep1)
	bystander := e.SeedPerson(t, "Tomas Ricci", &e.Rep1)

	if _, err := e.store.AddCommitment(e.rep1Ctx, planClock, weeklyplan.NewCommitment{
		Label: "Chase Anna Weber", LinkedRecordType: "person", LinkedRecordID: subject,
	}); err != nil {
		t.Fatal(err)
	}
	kept, err := e.store.AddCommitment(e.rep1Ctx, planClock, weeklyplan.NewCommitment{
		Label: "Send Tomas Ricci the pricing", LinkedRecordType: "person", LinkedRecordID: bystander,
	})
	if err != nil {
		t.Fatal(err)
	}
	// An unlinked commitment too: the statement selects on the link, so a
	// version that dropped its WHERE would take this one as well.
	unlinked, err := e.store.AddCommitment(e.rep1Ctx, planClock, weeklyplan.NewCommitment{
		Label: "Write the Q3 territory plan",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), subject, "subject request"); err != nil {
		t.Fatal(err)
	}

	plan, err := e.store.Current(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[ids.UUID]weeklyplan.Commitment{}
	for _, c := range plan.Commitments {
		byID[c.ID] = c
	}
	if got := byID[kept.ID]; got.Label != "Send Tomas Ricci the pricing" {
		t.Errorf("the bystander's commitment reads %q — one subject's erasure took another "+
			"person's row with it", got.Label)
	}
	if got := byID[kept.ID]; got.LinkedRecordID != bystander {
		t.Errorf("the bystander's link was cleared, so the erasure is selecting on more than " +
			"the subject it was asked about")
	}
	if got := byID[unlinked.ID]; got.Label != "Write the Q3 territory plan" {
		t.Errorf("an unlinked commitment reads %q — the statement lost its WHERE", got.Label)
	}
}

// A commitment may only link a record the rep can already open.
//
// The case that makes this real is CAPTURE PRIVACY, not row scope. Person,
// organization, lead, deal and project are identity tables: workspace-readable
// by design, so their owner arm renders TRUE for every seat and a colleague's
// contact is not hidden from anybody. What IS hidden is an unpromoted row a
// connector invented — visibility='owner' until a human promotes it — and that
// one does not yield even to row_scope=all.
//
// So the link is not an existence oracle over ordinary contacts; it would be a
// laundering path for the one class of identity row the workspace genuinely
// cannot read. A rep pastes the id, the commitment stores it, and their plan
// hands back a row from a colleague's unpromoted inbox.
func TestACommitmentCannotLinkAnUnpromotedContact(t *testing.T) {
	e := setupPlan(t)
	// The state a connector leaves a contact in: rep3's alone until promoted.
	private := e.SeedPerson(t, "A stranger's contact", &e.Rep3)
	e.MakeCapturePrivate(t, "person", private, e.Rep3)

	rep := boundedRep(e)
	_, err := e.store.AddCommitment(rep, planClock, weeklyplan.NewCommitment{
		Label: "Find out who this is", LinkedRecordType: "person", LinkedRecordID: private,
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("linking an unreadable person got %v, wanted not found — 403 would confirm "+
			"the record exists, which is the half the probe is for", err)
	}

	// And nothing was written. A refusal that rolled back only the link would
	// leave the rep's typed label standing on their plan.
	plan, err := e.store.Current(rep, planClock)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal(err)
	}
	if err == nil && len(plan.Commitments) != 0 {
		t.Errorf("the refused commitment left %d rows behind", len(plan.Commitments))
	}
}

// The admit case. Without it the refusal above passes on a probe that refuses
// EVERY link, which would make the feature useless while every test stayed
// green — the shape three security tests in this tree have already taken.
//
// A promoted contact of the rep's own, so the pair differs in exactly the
// property under test: both are people, both are seeded the same way, and only
// visibility separates them.
func TestACommitmentLinksARecordTheRepCanSee(t *testing.T) {
	e := setupPlan(t)
	mine := e.SeedPerson(t, "My own contact", &e.Rep1)

	out, err := e.store.AddCommitment(boundedRep(e), planClock, weeklyplan.NewCommitment{
		Label: "Call them Thursday", LinkedRecordType: "person", LinkedRecordID: mine,
	})
	if err != nil {
		t.Fatalf("linking a person the rep owns was refused: %v", err)
	}
	if out.LinkedRecordID != mine {
		t.Errorf("the link came back as %s, wanted %s", out.LinkedRecordID, mine)
	}
}

// An unknown link type is refused as a request error, before any statement is
// built. The type is interpolated into SQL by the visibility probe, so a word
// off a request body reaching that far is the injection this map exists to
// stop — and a 500 would say the installation broke rather than that the
// request was wrong.
func TestAnUnknownLinkTypeIsRefused(t *testing.T) {
	e := setupPlan(t)

	_, err := e.store.AddCommitment(e.rep1Ctx, planClock, weeklyplan.NewCommitment{
		Label:            "Link something that is not a record",
		LinkedRecordType: "app_user",
		LinkedRecordID:   e.Rep3,
	})
	if err == nil {
		t.Fatal("an unknown link type was accepted")
	}
	if errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an unknown type answered not-found, so it reached the row probe: %v", err)
	}
}

// The retention sweep's person/anonymize reaches a commitment too.
//
// The gate personscrub_test.go proves the two acts clear the same TABLES; it
// reads statements, not results, so it cannot tell that the anonymize path
// actually runs this one. That gap matters here more than usual: an operator
// told a record was anonymized has been told the subject's data is gone, and
// this table is reached by neither a schema cascade nor any person-keyed clause.
//
// Driven through the REAL retention engine and the seeded
// person/no_consent_no_deal policy rather than by calling the action directly,
// so what is proven is the path an installation actually runs.
func TestTheAnonymizeSweepRedactsACommitmentNamingTheSubject(t *testing.T) {
	e := setupPlan(t)
	integration.SeedRetentionPolicies(t, e.Env)

	// Past the 730-day window, with no consent and no deal role — the
	// selector's whole definition of an unattached contact.
	subject := ids.NewV7()
	e.WsExec(t, `INSERT INTO person (id, full_name, first_name, last_name, source, captured_by, created_at)
		VALUES ($1, 'Old Contact', 'Old', 'Contact', 'manual', 'human:x', now() - interval '800 days')`,
		subject)

	commitment, err := e.store.AddCommitment(e.rep1Ctx, planClock, weeklyplan.NewCommitment{
		Label:            "Ask Old Contact whether they ever signed",
		LinkedRecordType: "person",
		LinkedRecordID:   subject,
	})
	if err != nil {
		t.Fatalf("writing the commitment: %v", err)
	}

	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := svc.EvaluateInstallation(integration.RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("retention pass: %v", err)
	}

	// Read under the owner connection rather than through the store: the person
	// row is archived by now, and the point is what the TABLE holds.
	var label, linkType string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT label, coalesce(linked_record_type, '') FROM weekly_plan_commitment WHERE id = $1`,
			commitment.ID).Scan(&label, &linkType)
	}); err != nil {
		t.Fatal(err)
	}

	if label == "Ask Old Contact whether they ever signed" {
		t.Errorf("the commitment still reads %q — an operator was told this subject "+
			"was anonymized while a colleague's plan went on naming them", label)
	}
	if linkType != "" {
		t.Errorf("the link still points at a %s, so the row still says which person it was", linkType)
	}
}
