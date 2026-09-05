// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// A self-only kind's identity-keyed paths, over a real Postgres.
//
// decidable says a self-only proposal belongs to ONE member: only they may read
// it and only they may decide it. The staging engine's matching statements did
// not know that. They matched on kind, target and either the logical identity
// or the diff hash — so two members proposing the same thing landed on ONE row,
// expired each other's pending proposals, and inherited each other's declines.
//
// These are Postgres tests because the boundary IS the predicate: the clause
// lives in SQL, and a Go double would be asserting against its own idea of what
// the statement matches.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forMember is the connector principal that stages a card ON BEHALF OF one
// member — the shape every self-only kind is staged in. A human staging
// directly carries no OnBehalfOf, and a self-only row without one is decidable
// by nobody (decidable fails closed), so it is not the shape under test.
func (e *stagingEnv) forMember(member ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:vcard",
		UserID: member, OnBehalfOf: member,
		Permissions: principal.Permissions{RoleKeys: []string{"admin"}},
	})
}

// secondMember seeds another seat in the same workspace.
func (e *stagingEnv) secondMember(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Second')`,
		id, "second-"+id.String()+"@st.test"); err != nil {
		t.Fatal(err)
	}
	return id
}

// nameOnlyCard is the card the ticket's reachable case is about: two members
// upload a card for two DIFFERENT people who happen to share a name, so the
// logical identity collides while the subjects do not.
func nameOnlyCard() StageInput {
	const sharedName = "Jan Mueller"
	return StageInput{
		Kind:           "vcard_create",
		ProposedChange: []byte(`{"full_name":"` + sharedName + `"}`),
		DiffHash:       "hash-of-" + sharedName,
		Identity:       []byte(`{"full_name":"` + sharedName + `"}`),
		Summary:        "Add " + sharedName + "?",
		JoinPending:    true,
	}
}

// subjectOf reads the member a proposal was staged for, and nil for one staged
// on nobody's behalf — the nullable case the scope's IS NOT DISTINCT FROM
// exists for, which a non-pointer scan could not even report.
func (e *stagingEnv) subjectOf(t *testing.T, id ids.ApprovalID) *ids.UUID {
	t.Helper()
	var subject *ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT on_behalf_of FROM approval WHERE id = $1`, id).Scan(&subject); err != nil {
		t.Fatalf("reading the proposal's subject: %v", err)
	}
	return subject
}

// Two members, one identity, two proposals. Joining them would hand the second
// member a row the first owns — invisible to them under decidable's own gate,
// and undecidable by them, with nothing saying why their card never appeared.
func TestTwoMembersStagingOneIdentityKeepTheirOwnProposal(t *testing.T) {
	e := setupStaging(t)
	other := e.secondMember(t)
	card := nameOnlyCard()

	first, staged, err := e.svc.StageUnlessDeclined(e.forMember(e.rep), card)
	if err != nil || !staged {
		t.Fatalf("staging the first member's card: staged=%v err=%v", staged, err)
	}
	second, staged, err := e.svc.StageUnlessDeclined(e.forMember(other), card)
	if err != nil || !staged {
		t.Fatalf("staging the second member's card: staged=%v err=%v", staged, err)
	}

	if first == second {
		t.Fatal("both members' cards landed on ONE proposal: the second member cannot read or decide a " +
			"row staged for the first, so their card is in the queue for nobody")
	}
	if got := e.subjectOf(t, first); got == nil || *got != e.rep {
		t.Errorf("the first proposal is staged for %v, not the member who uploaded it", got)
	}
	if got := e.subjectOf(t, second); got == nil || *got != other {
		t.Errorf("the second proposal is staged for %v, not the member who uploaded it", got)
	}
}

// Supersession expires the proposals a new one replaces. Under one identity and
// two members it must expire only the proposer's own: a colleague's pending
// card is not a stale version of this one.
func TestSupersessionUnderOneIdentityLeavesTheOtherMembersProposalPending(t *testing.T) {
	e := setupStaging(t)
	other := e.secondMember(t)

	theirs, _, err := e.svc.StageUnlessDeclined(e.forMember(other), nameOnlyCard())
	if err != nil {
		t.Fatalf("staging the other member's card: %v", err)
	}
	// Same identity, different payload, so this is a NEW proposal that
	// supersedes rather than a join.
	restaged := nameOnlyCard()
	restaged.ProposedChange = []byte(`{"full_name":"Jan Mueller","title":"Buyer"}`)
	restaged.DiffHash = "hash-of-jan-with-title"
	mine, _, err := e.svc.StageUnlessDeclined(e.forMember(e.rep), restaged)
	if err != nil {
		t.Fatalf("staging this member's card: %v", err)
	}

	if got := e.statusOf(t, theirs); got != "pending" {
		t.Errorf("the other member's card is %q: one member's re-proposal expired a colleague's card, "+
			"and the sweep will not even record it as unactioned", got)
	}
	if got := e.statusOf(t, mine); got != "pending" {
		t.Errorf("this member's own card is %q rather than pending", got)
	}
}

// The declined memory is one member's refusal. Applied across members it
// refuses a colleague's proposal, and the colleague sees no offer and no reason
// for its absence.
func TestOneMembersDeclineDoesNotRefuseAnotherMembersProposal(t *testing.T) {
	e := setupStaging(t)
	other := e.secondMember(t)
	card := nameOnlyCard()

	theirs, _, err := e.svc.StageUnlessDeclined(e.forMember(other), card)
	if err != nil {
		t.Fatalf("staging the other member's card: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE approval SET status = 'rejected', decided_at = now(), decided_by = $2 WHERE id = $1`,
		theirs, other); err != nil {
		t.Fatalf("recording the other member's refusal: %v", err)
	}

	mine, staged, err := e.svc.StageUnlessDeclined(e.forMember(e.rep), card)
	if err != nil {
		t.Fatalf("staging this member's card: %v", err)
	}
	if !staged {
		t.Fatal("this member's card was refused by a colleague's decline: the two are different cards for " +
			"different people who share a name, and this member is never told why theirs never arrived")
	}
	if got := e.subjectOf(t, mine); got == nil || *got != e.rep {
		t.Errorf("the staged proposal belongs to %v rather than the member who uploaded it", got)
	}
}

// The narrowing must not over-reach. A SHARED kind is the inbox working as
// designed — a manager triages what a rep staged — so two members proposing the
// same change still land on one row and still inherit each other's decisions.
// Without this assertion the fix above could be "scope everything" and nothing
// would say the inbox had stopped being shared.
func TestASharedKindStillMatchesAcrossMembers(t *testing.T) {
	e := setupStaging(t)
	other := e.secondMember(t)
	target := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO organization (id, display_name, source, captured_by)
		VALUES ($1, 'Gitex', 'gmail:seed', 'connector:gmail')`, target); err != nil {
		t.Fatal(err)
	}
	rename := StageInput{
		Kind:           "org_name_promotion",
		ProposedChange: []byte(`{"proposed_name":"Gitex Global"}`),
		DiffHash:       "one-shared-proposal",
		TargetType:     "organization",
		TargetID:       target,
		Summary:        "Rename Gitex to Gitex Global?",
		JoinPending:    true,
	}

	first, _, err := e.svc.StageUnlessDeclined(e.forMember(e.rep), rename)
	if err != nil {
		t.Fatalf("staging the first proposal: %v", err)
	}
	second, _, err := e.svc.StageUnlessDeclined(e.forMember(other), rename)
	if err != nil {
		t.Fatalf("staging the second proposal: %v", err)
	}

	if first != second {
		t.Error("two members proposing one rename produced two inbox rows: a shared kind matches across " +
			"members by design, and the subject narrowing has reached a kind it is not for")
	}
}

// on_behalf_of is nullable, and `=` against NULL is NULL rather than true. A
// scope written with `=` would therefore match NOTHING for a proposal staged on
// nobody's behalf — the join never joins, the supersede never supersedes and
// the decline is never remembered, each failing silently as a control that
// quietly does nothing.
//
// The decline is the one with teeth: forgotten, the next pass re-offers exactly
// what a human refused.
func TestASubjectlessProposalStillRemembersItsOwnDecline(t *testing.T) {
	e := setupStaging(t)
	card := nameOnlyCard()
	// A HUMAN principal carries no OnBehalfOf, so the row it stages holds NULL —
	// the case `=` cannot see.
	refused, _, err := e.svc.StageUnlessDeclined(e.as(), card)
	if err != nil {
		t.Fatalf("staging the first card: %v", err)
	}
	if got := e.subjectOf(t, refused); got != nil {
		t.Fatalf("this proposal was staged for %s, so it is not the subjectless case under test", *got)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE approval SET status = 'rejected', decided_at = now(), decided_by = $2 WHERE id = $1`,
		refused, e.rep); err != nil {
		t.Fatalf("recording the refusal: %v", err)
	}

	if _, staged, err := e.svc.StageUnlessDeclined(e.as(), card); err != nil || staged {
		t.Errorf("the refused card was staged again (staged=%v err=%v): a scope comparing a NULL subject "+
			"with `=` matches nothing, so the memory forgets a refusal instead of holding it", staged, err)
	}
}
