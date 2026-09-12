// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A passport carries its granting human's UserID, so a check for one admits an
// agent. Two reads relied on such a check to stay human-only.
//
// The pattern is worth naming because it looks like a gate and is not:
//
//	if actor, ok := principal.Actor(ctx); !ok || actor.UserID.IsZero() { ... }
//
// It asks "is anybody behind this call", which an agent answers YES to using
// the id of the contact who minted it. Each read then went on to return that
// contact's own standing — what they dismissed, who refused them — to a
// credential acting on their behalf. auth.RequireHuman is what tells the two
// apart, and each site now says so where the check is.
//
// introductions/routestate.go named getContactGraph's human-only annotation as
// its protection, which made the answer depend on which door a read arrived
// through rather than on what it discloses. That protection was also narrower
// than it looked: an agent already assembles a Contact360 today, through
// prep_for_meeting → the meeting brief → AssembleScoped, so the dismissal guard
// is live behaviour rather than defence in depth.
//
// The third case in this file is not about agents at all: the employment edge
// returned an employer's name to a caller holding no company grant, which
// the function's own doc comment already claimed it did not.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// routeStatePerms holds every grant RouteStates checks before its human arm runs, so
// a refusal in this fixture is that arm and never a missing grant.
var routeStatePerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":      {Read: true},
		"introduction": {Read: true},
		"relationship": {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

// TestTheIntroductionLedgerRefusesAnAgentCarryingItsHumansID covers the read
// that reports which introductions the caller already asked for and which they
// were refused.
//
// The human is admitted and the agent is not, over ONE fixture with ONE set of
// permissions: the two contexts differ in principal type and in nothing the
// test chose, so the refusal cannot be a grant the agent was never given.
func TestTheIntroductionLedgerRefusesAnAgentCarryingItsHumansID(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Marit Vermittelt", &e.Rep1)
	store := introductions.NewStore(e.DB(), time.Now)

	human := e.As(e.Rep1, []ids.UUID{e.Team1}, routeStatePerms)
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, routeStatePerms)

	// The positive control comes first: a fixture that refuses everybody would
	// pass the agent assertion below while proving nothing.
	if _, err := store.RouteStates(human, ids.From[ids.ContactKind](contact)); err != nil {
		t.Fatalf("the granting human's own read = %v, want the ledger", err)
	}

	_, err := store.RouteStates(agent, ids.From[ids.ContactKind](contact))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent read the introduction ledger → %v, want ErrPermissionDenied — "+
			"it reports which introductions its human has open and which they were "+
			"refused, so an agent reading it learns its human was turned down", err)
	}
}

// TestRequireHumanIsWhatSeparatesAnAgentFromItsHuman states the premise the two
// guards now rest on, as a fact rather than an assumption: the agent and its
// granting human carry the SAME UserID, so an id check cannot separate them and
// auth.RequireHuman is what does.
//
// Asserted directly because it is the premise, not the behaviour: the guards
// themselves are covered through their real callers above and below.
func TestRequireHumanIsWhatSeparatesAnAgentFromItsHuman(t *testing.T) {
	e := Setup(t)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, routeStatePerms)
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, routeStatePerms)

	humanActor, ok := principal.Actor(human)
	if !ok {
		t.Fatal("the human fixture carries no actor")
	}
	agentActor, ok := principal.Actor(agent)
	if !ok {
		t.Fatal("the agent fixture carries no actor")
	}

	// The defect, stated. If this ever stops being true, the guards below are
	// answering a different question and this file should be re-read.
	if agentActor.UserID != humanActor.UserID {
		t.Fatalf("the agent resolved to user %s and its granting human to %s — a passport "+
			"is supposed to carry its human's id, and this suite's premise is that an "+
			"id check therefore cannot refuse one", agentActor.UserID, humanActor.UserID)
	}
	if agentActor.UserID.IsZero() {
		t.Fatal("the agent carries no user id at all, so the id check WOULD refuse it " +
			"and the fixture is not modelling the case these guards are about")
	}

	if err := auth.RequireHuman(human); err != nil {
		t.Errorf("auth.RequireHuman refused the granting human → %v, want admission", err)
	}
	if err := auth.RequireHuman(agent); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("auth.RequireHuman admitted an agent → %v, want ErrPermissionDenied — "+
			"it is the only thing between a passport and its human's own standing", err)
	}
}

// TestAnEmployerNameNeedsTheCompanyGrant covers the object half of the
// employment edge's answer, through the assembled page.
//
// companyName's doc says a name the caller cannot read "is simply absent",
// and its statement selected on id and archived_at alone — so a caller holding
// contact and relationship but no company grant read employer names through
// the edge. The edge itself still shows: an employment they may see is a true
// fact, and only the company on the far side of it is a separate question.
func TestAnEmployerNameNeedsTheCompanyGrant(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Vertraulich GmbH", &e.Rep1)
	contact := e.SeedContact(t, "Pia Angestellt", &e.Rep1)
	e.WsExec(t, `
		INSERT INTO relationship (id, kind, contact_id, company_id, is_current_primary, source, captured_by)
		VALUES ($1, 'employment', $2, $3, true, 'manual', 'human:x')`, ids.NewV7(), contact, company)

	grants := func(withCompany bool) principal.Permissions {
		objects := map[string]principal.ObjectGrant{
			"contact": {Read: true}, "relationship": {Read: true}, "activity": {Read: true},
		}
		if withCompany {
			objects["company"] = principal.ObjectGrant{Read: true}
		}
		return principal.Permissions{
			RoleKeys: []string{"rep"}, Objects: objects, RowScope: principal.RowScopeAll,
		}
	}

	employer := func(withCompany bool) (string, bool) {
		t.Helper()
		ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, grants(withCompany))
		page, err := contactRoomService(e).Assemble(ctx, ids.From[ids.ContactKind](contact))
		if err != nil {
			t.Fatalf("assembling the page (company grant=%v): %v", withCompany, err)
		}
		if page.Employments == nil || len(page.Employments.Data) != 1 {
			t.Fatalf("the page carried %+v employments, want exactly the seeded one — "+
				"an assertion about a name on a row that is not there passes for the "+
				"wrong reason", page.Employments)
		}
		row := page.Employments.Data[0]
		if row.CompanyName == nil {
			return "", false
		}
		return *row.CompanyName, true
	}

	// The control: with the grant the name is there, so its absence below is
	// the refusal rather than an empty fixture.
	if got, ok := employer(true); !ok || got != "Vertraulich GmbH" {
		t.Fatalf("a caller holding company.read saw name=%q present=%v, "+
			"want the employer", got, ok)
	}
	if got, ok := employer(false); ok {
		t.Errorf("a caller holding no company grant read the employer name %q "+
			"through the employment edge", got)
	}
}

// TestAnAgentDoesNotConsumeItsHumansDismissal drives momentDismissed through
// the real assembly rather than asserting auth.RequireHuman in isolation.
//
// This is the arm that matters: an agent CAN already assemble a contact page
// today, through prep_for_meeting → the meeting brief → Contact360. So the guard
// is live behaviour and not defence in depth, and a test that only proved
// RequireHuman refuses agents would leave the assembly free to stop calling it.
//
// The fixture dismisses as the HUMAN and then reads the same page twice. The
// human's moment is gone; the agent's is still there, because a dismissal is
// one contact's screen and a passport reading on their behalf is not them.
func TestAnAgentDoesNotConsumeItsHumansDismissal(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Rune Verstummt", &e.Rep1)

	perms := principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true}, "activity": {Read: true, Create: true},
			"relationship": {Read: true}, "company": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	}
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, perms)
	svc := contactRoomService(e)

	// A moment has to FIRE before a dismissal of it means anything. Whatever
	// the ladder picks is fine — the test is about the dismissal, not about
	// which rung won — so the fixture reads the moment rather than asserting a
	// particular kind.
	first, err := svc.Assemble(human, ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("assembling the page: %v", err)
	}
	if first.Moment == nil {
		t.Fatal("the page opened on no moment at all, so there is nothing to dismiss " +
			"and the arms below would both pass vacuously")
	}
	claim, fingerprint := first.Moment.ClaimKey, first.Moment.EvidenceFingerprint

	e.WsExec(t, `
		INSERT INTO contact_moment_dismissal (user_id, contact_id, claim_key, evidence_fingerprint)
		VALUES ($1, $2, $3, $4)`, e.Rep1, contact, claim, fingerprint)

	// The human put it away, so their own page must not still offer it.
	mine, err := svc.Assemble(human, ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("re-assembling the human's page: %v", err)
	}
	if mine.Moment != nil && mine.Moment.ClaimKey == claim {
		t.Fatalf("the human's own dismissal was ignored — the page still opens on %q, "+
			"so this fixture is not exercising the dismissal path at all", claim)
	}

	// The agent's is untouched: it never had a screen to put anything away on.
	theirs, err := svc.Assemble(agent, ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("assembling the agent's page: %v", err)
	}
	if theirs.Moment == nil || theirs.Moment.ClaimKey != claim {
		t.Errorf("an agent's page lost the moment its granting human dismissed "+
			"(got %+v, want claim %q) — a dismissal belongs to a contact's screen, "+
			"and a passport reading on their behalf consumed it", theirs.Moment, claim)
	}
}
