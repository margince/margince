// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The vCard near-match review loop against a real database: the import
// refuses the near-match, the refusal becomes ONE durable proposal however
// often the same card is uploaded, approving it creates the person through
// the real writer, and a decline is remembered against the card's identity.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The existing contact's address and the card's: SAME name, different
// address. An exact email match is merged outright; the near-match the
// import refuses to create is the name collision with no shared key.
const (
	existingContactEmail = "anna.weber@example.com"
	reviewedCardEmail    = "a.weber@webers.example"
)

// reviewedCard is the card that collides on the name with a person the
// workspace already holds — the shape the import refuses to create.
func reviewedCard() people.VCardEntry {
	return people.VCardEntry{
		FullName:     "Anna Weber",
		Organization: "Weber Consulting",
		Emails:       []people.VCardChannel{{Value: reviewedCardEmail, Kind: "work"}},
	}
}

// seedNearMatch writes the existing contact through the real writer and
// answers the import's review verdict for the given card, which must
// resemble them by name.
func seedNearMatch(ctx context.Context, t *testing.T, e *integration.Env, card people.VCardEntry) *ids.PersonID {
	t.Helper()
	if _, err := e.People.CreatePerson(ctx, people.CreatePersonInput{
		FullName: "Anna Weber", Source: "ui",
		Emails: []people.PersonEmailInput{{Email: existingContactEmail, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("seeding the existing contact: %v", err)
	}
	results, err := e.People.ImportVCards(ctx, []people.VCardEntry{card})
	if err != nil {
		t.Fatalf("importing the near-match card: %v", err)
	}
	if len(results) != 1 || results[0].Outcome != people.VCardNeedsReview {
		t.Fatalf("import outcome = %+v, want the one needs_review refusal", results)
	}
	return results[0].PersonID
}

func TestAVCardNearMatchBecomesOneDurableProposal(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	candidate := seedNearMatch(ctx, t, e, reviewedCard())
	if candidate == nil {
		t.Fatal("the import named no visible candidate for an admin importer")
	}

	stage := vcardCreateStager(e.Pool)
	if err := stage(ctx, reviewedCard(), candidate); err != nil {
		t.Fatalf("staging the review: %v", err)
	}
	// The same card again — a re-uploaded file — joins the pending question
	// rather than asking it twice.
	if err := stage(ctx, reviewedCard(), candidate); err != nil {
		t.Fatalf("re-staging the review: %v", err)
	}

	svc := approvalsServiceWithEffects(e.Pool)
	status := "pending"
	kind := vcardCreateKind
	rows, _, err := svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing the staged reviews: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("pending vcard_create proposals = %d, want the one joined question", len(rows))
	}
	// The uploaded card is one member's own address book: a colleague with
	// every create grant still reads nothing. Self-only is what keeps a
	// third party's contact data from becoming workspace-readable by upload.
	colleague := e.As(e.Rep1, nil, integration.AdminPerms)
	overShoulder, _, err := svc.ListWire(colleague, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing as a colleague: %v", err)
	}
	if len(overShoulder) != 0 {
		t.Errorf("a colleague read %d of the importer's card proposals, want none", len(overShoulder))
	}

	if _, err := svc.Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id)), true, nil); err != nil {
		t.Fatalf("approving the create: %v", err)
	}
	var created int
	if err := e.Pool.QueryRow(ctx,
		`SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		  WHERE lower(pe.email) = $1`, reviewedCardEmail).Scan(&created); err != nil {
		t.Fatalf("counting people holding the card's address: %v", err)
	}
	if created != 1 {
		t.Errorf("people holding the card's own address %s = %d, want exactly the approved create", reviewedCardEmail, created)
	}
	// The other half of the release: the employer edge, through to the
	// organization the card named.
	var employed int
	if err := e.Pool.QueryRow(ctx, `
		SELECT count(*) FROM relationship r
		  JOIN organization o ON o.id = r.organization_id
		  JOIN person_email pe ON pe.person_id = r.person_id
		 WHERE r.kind = 'employment' AND lower(pe.email) = $1
		   AND o.display_name = 'Weber Consulting'`, reviewedCardEmail).Scan(&employed); err != nil {
		t.Fatalf("counting the employment edge: %v", err)
	}
	if employed != 1 {
		t.Errorf("employment edges to Weber Consulting = %d, want the card's one", employed)
	}
}

// orgLessReviewedCard is a near-match with no ORG line at all — an email
// still gives it real addressing, but the identity asserts organization too
// (as the empty string), and the staged payload must carry that same key or
// the engine's containment check refuses the mismatch.
func orgLessReviewedCard() people.VCardEntry {
	return people.VCardEntry{
		FullName: "Anna Weber",
		Emails:   []people.VCardChannel{{Value: reviewedCardEmail, Kind: "work"}},
	}
}

func TestACardNamingNoCompanyCanStillBeStaged(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	candidate := seedNearMatch(ctx, t, e, orgLessReviewedCard())

	stage := vcardCreateStager(e.Pool)
	if err := stage(ctx, orgLessReviewedCard(), candidate); err != nil {
		t.Fatalf("staging a card naming no company: %v", err)
	}

	// A staging error nil does not, by itself, prove a decidable row exists —
	// StageUnlessDeclined can also settle onto a prior decline with no new
	// row. The reader this bug affected needs the review IN THEIR QUEUE.
	svc := approvalsServiceWithEffects(e.Pool)
	status, kind := "pending", vcardCreateKind
	rows, _, err := svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing the staged review: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("pending vcard_create proposals = %d, want the one org-less review", len(rows))
	}

	// The release half is the one org-less cards exercise differently: no
	// company to attach means no employment edge, which is a distinct code
	// path from the org-present create the sibling test already covers.
	if _, err := svc.Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id)), true, nil); err != nil {
		t.Fatalf("approving the org-less create: %v", err)
	}
	var created, employed int
	if err := e.Pool.QueryRow(ctx,
		`SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		  WHERE lower(pe.email) = $1`, reviewedCardEmail).Scan(&created); err != nil {
		t.Fatalf("counting people holding the card's address: %v", err)
	}
	if created != 1 {
		t.Errorf("people holding %s = %d, want exactly the approved create", reviewedCardEmail, created)
	}
	if err := e.Pool.QueryRow(ctx, `
		SELECT count(*) FROM relationship r
		  JOIN person_email pe ON pe.person_id = r.person_id
		 WHERE r.kind = 'employment' AND lower(pe.email) = $1`, reviewedCardEmail).Scan(&employed); err != nil {
		t.Fatalf("counting employment edges: %v", err)
	}
	if employed != 0 {
		t.Errorf("employment edges for a card naming no company = %d, want none", employed)
	}
}

// vacuousReviewedCard names a person and nothing else — no email, no
// organization. Without a subject folded into the identity, this would
// collapse to the bare name alone — exactly what two DIFFERENT workspace
// members could independently produce for two DIFFERENT people who share
// that name.
func vacuousReviewedCard() people.VCardEntry {
	return people.VCardEntry{FullName: "Priya Raghunathan"}
}

// A card's identity is scoped to the SUBJECT who staged it (vcard_create is
// self-only: nobody but the importer may read or decide their own upload),
// and the engine's identity-keyed supersession carries no subject filter of
// its own — it trusts the caller's identity to already be collision-proof.
// A name-only card is the sharpest version of that trust failing: nothing
// but staged_by distinguishes "Priya Raghunathan, no email, no company" in
// one member's address book from another member's. Two members staging
// that same bare name must not see one of their reviews withdrawn out from
// under them.
//
// The two cards also carry different candidates, so their PAYLOADS differ
// (and so join on neither the same diff hash nor the same row) independent
// of staged_by — isolating the identity-keyed supersession path this test
// is actually about from the engine's separate diff-hash join step, which
// this kind's staged_by field also happens to disambiguate but which no
// self-only kind's join is scoped by on its own.
func TestAVacuousCardDoesNotSupersedeAnotherSubjectsPendingReview(t *testing.T) {
	e := integration.Setup(t)
	adminCtx := e.Admin()
	repCtx := e.As(e.Rep1, nil, integration.AdminPerms)

	adminCandidate, err := e.People.CreatePerson(adminCtx, people.CreatePersonInput{FullName: "A Different Priya", Source: "ui"})
	if err != nil {
		t.Fatalf("seeding the admin's candidate: %v", err)
	}
	adminCandidateID := ids.From[ids.PersonKind](ids.UUID(adminCandidate.Id))

	stage := vcardCreateStager(e.Pool)
	if err := stage(adminCtx, vacuousReviewedCard(), &adminCandidateID); err != nil {
		t.Fatalf("admin staging the vacuous card: %v", err)
	}
	if err := stage(repCtx, vacuousReviewedCard(), nil); err != nil {
		t.Fatalf("rep staging the same vacuous card: %v", err)
	}

	svc := approvalsServiceWithEffects(e.Pool)
	status, kind := "pending", vcardCreateKind
	adminRows, _, err := svc.ListWire(adminCtx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing the admin's reviews: %v", err)
	}
	if len(adminRows) != 1 {
		t.Errorf("admin's pending vcard_create reviews = %d, want the one they staged — a colleague's later upload must not withdraw it", len(adminRows))
	}
	repRows, _, err := svc.ListWire(repCtx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing the rep's reviews: %v", err)
	}
	if len(repRows) != 1 {
		t.Errorf("rep's pending vcard_create reviews = %d, want the one they staged", len(repRows))
	}
}

// The two tests above give each member a different candidate so their
// PAYLOADS differ regardless of staged_by, isolating the identity-keyed
// supersession path. This one closes the gap that leaves: the SAME
// candidate (both nil — neither importer sees a near-match) means the only
// thing that can still make two members' payloads diverge is staged_by
// itself, which is exactly what the engine's separate diff-hash JOIN step
// would otherwise ignore.
func TestATwoMembersCardsWithNoCandidateDoNotJoinIntoOneReview(t *testing.T) {
	e := integration.Setup(t)
	adminCtx := e.Admin()
	repCtx := e.As(e.Rep1, nil, integration.AdminPerms)

	stage := vcardCreateStager(e.Pool)
	if err := stage(adminCtx, coworkerNamedCard(), nil); err != nil {
		t.Fatalf("admin staging their card: %v", err)
	}
	if err := stage(repCtx, coworkerNamedCard(), nil); err != nil {
		t.Fatalf("rep staging the identical card: %v", err)
	}

	svc := approvalsServiceWithEffects(e.Pool)
	status, kind := "pending", vcardCreateKind
	adminRows, _, err := svc.ListWire(adminCtx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing the admin's reviews: %v", err)
	}
	if len(adminRows) != 1 {
		t.Errorf("admin's pending vcard_create reviews = %d, want the one they staged — the rep's identical upload must not join it", len(adminRows))
	}
	repRows, _, err := svc.ListWire(repCtx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing the rep's reviews: %v", err)
	}
	if len(repRows) != 1 {
		t.Errorf("rep's pending vcard_create reviews = %d, want their own review, not the admin's joined row (which self-only hides from them)", len(repRows))
	}
}

// coworkerNamedCard names a person AND a company — real addressing, not the
// bare-name shape above — because the fix under test is not "vacuous cards
// get special treatment", it is "every card's identity is subject-scoped".
// A name and an employer are both facts a coworker could read off a business
// card or a signature block, so a value-derived identity built from card
// fields alone is guessable no matter how many of them are populated.
func coworkerNamedCard() people.VCardEntry {
	return people.VCardEntry{FullName: "Jan Kowalski", Organization: "Acme GmbH"}
}

// The harder case behind the one above: two DIFFERENT people who both work
// at the same company and share a name are not a hypothetical the fix
// happens to miss — they are exactly what an attacker with person:create
// need only observe (or guess) to reproduce another member's card byte for
// byte, if organization alone still discriminated the identity.
func TestATwoFieldCardDoesNotSupersedeAnotherSubjectsPendingReview(t *testing.T) {
	e := integration.Setup(t)
	adminCtx := e.Admin()
	repCtx := e.As(e.Rep1, nil, integration.AdminPerms)

	adminCandidate, err := e.People.CreatePerson(adminCtx, people.CreatePersonInput{FullName: "A Different Jan Kowalski", Source: "ui"})
	if err != nil {
		t.Fatalf("seeding the admin's candidate: %v", err)
	}
	adminCandidateID := ids.From[ids.PersonKind](ids.UUID(adminCandidate.Id))

	stage := vcardCreateStager(e.Pool)
	if err := stage(adminCtx, coworkerNamedCard(), &adminCandidateID); err != nil {
		t.Fatalf("admin staging their card: %v", err)
	}
	if err := stage(repCtx, coworkerNamedCard(), nil); err != nil {
		t.Fatalf("rep staging the same name-and-company card: %v", err)
	}

	svc := approvalsServiceWithEffects(e.Pool)
	status, kind := "pending", vcardCreateKind
	adminRows, _, err := svc.ListWire(adminCtx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing the admin's reviews: %v", err)
	}
	if len(adminRows) != 1 {
		t.Errorf("admin's pending vcard_create reviews = %d, want the one they staged — a coworker sharing this card's name and company must not withdraw it", len(adminRows))
	}
	repRows, _, err := svc.ListWire(repCtx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("listing the rep's reviews: %v", err)
	}
	if len(repRows) != 1 {
		t.Errorf("rep's pending vcard_create reviews = %d, want the one they staged", len(repRows))
	}
}

func TestADeclinedVCardReviewIsNotReAsked(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	candidate := seedNearMatch(ctx, t, e, reviewedCard())

	stage := vcardCreateStager(e.Pool)
	if err := stage(ctx, reviewedCard(), candidate); err != nil {
		t.Fatalf("staging the review: %v", err)
	}
	svc := approvalsServiceWithEffects(e.Pool)
	status, kind := "pending", vcardCreateKind
	rows, _, err := svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("staged reviews = %d (err %v), want 1", len(rows), err)
	}
	if _, err := svc.Decide(ctx, ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id)), false, nil); err != nil {
		t.Fatalf("declining the create: %v", err)
	}

	// The same card in tomorrow's upload: the decline is remembered against
	// the card's identity, and the question is not asked again.
	if err := stage(ctx, reviewedCard(), candidate); err != nil {
		t.Fatalf("re-staging after the decline: %v", err)
	}
	rows, _, err = svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("re-listing: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("a declined card was re-proposed: %d pending rows", len(rows))
	}
}

// The modify-then-approve arm can rewrite the payload, and the generic edit
// gate only pins entity references — so the kind's own precheck must refuse
// an edit that dropped the card, while the proposal is still pending and
// re-decidable. Without it the approval would commit and create a person
// with no name.
func TestAnEditThatDropsTheCardIsRefusedWhileStillPending(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	candidate := seedNearMatch(ctx, t, e, reviewedCard())
	stage := vcardCreateStager(e.Pool)
	if err := stage(ctx, reviewedCard(), candidate); err != nil {
		t.Fatalf("staging the review: %v", err)
	}
	svc := approvalsServiceWithEffects(e.Pool)
	status, kind := "pending", vcardCreateKind
	rows, _, err := svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("staged reviews = %d (err %v), want 1", len(rows), err)
	}
	id := ids.From[ids.ApprovalKind](ids.UUID(rows[0].Id))

	gutted := []byte(`{"full_name":"anna weber","emails":"` + reviewedCardEmail + `"}`)
	if _, err := svc.DecideEdited(ctx, id, gutted); err == nil {
		t.Fatal("an edit with no card was approved")
	}
	after, _, err := svc.ListWire(ctx, approvals.ListInput{Status: &status, Kind: &kind, Limit: 10})
	if err != nil {
		t.Fatalf("re-listing: %v", err)
	}
	if len(after) != 1 {
		t.Errorf("the refused edit left %d pending rows, want the proposal still decidable", len(after))
	}
	var nameless int
	if err := e.Pool.QueryRow(ctx,
		`SELECT count(*) FROM person WHERE trim(full_name) = ''`).Scan(&nameless); err != nil {
		t.Fatal(err)
	}
	if nameless != 0 {
		t.Errorf("a person with no name exists: %d", nameless)
	}
}
