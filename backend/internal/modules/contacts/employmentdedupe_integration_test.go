// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/filedpairs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func mustEmployer(t *testing.T, e *dedupeEnv, name string) ids.CompanyID {
	t.Helper()
	c, err := e.store.CreateCompany(e.as(), CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("create company %q: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(c.Id))
}

func mustContact(t *testing.T, e *dedupeEnv, name string) ids.ContactID {
	t.Helper()
	c, err := e.store.CreateContact(e.as(), CreateContactInput{FullName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("create contact %q: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(c.Id))
}

func employ(t *testing.T, e *dedupeEnv, contact ids.ContactID, company ids.CompanyID) {
	t.Helper()
	if _, err := e.store.CreateRelationship(e.as(), CreateRelationshipInput{
		Kind: employmentKind, ContactID: &contact, CompanyID: &company, Source: "manual",
	}); err != nil {
		t.Fatalf("employ: %v", err)
	}
}

// A contact created from an attendee's name has no employer when the create's
// duplicate check runs. Giving it the employer of a same-name contact files the
// pair when the edge is written.
func TestEmploymentEdgeFilesTheSameEmployerTwin(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	acme := mustEmployer(t, e, "Acme Robotics GmbH")
	onFile := mustContact(t, e, "Marlene Wiesinger")
	employ(t, e, onFile, acme)
	// A one-letter variant is not an exact collision, so only the shared
	// employer lifts it over the review threshold.
	twin := mustContact(t, e, "Marlene Wiesingr")
	if pairs := openCandidatePairs(ctx, t, e, "contact", twin.UUID); len(pairs) != 0 {
		t.Fatalf("got %d pairs before the edge, want 0 — the employer must be the evidence", len(pairs))
	}

	employ(t, e, twin, acme)

	pairs := openCandidatePairs(ctx, t, e, "contact", twin.UUID)
	if len(pairs) != 1 || pairs[0].OtherID != onFile.String() {
		t.Fatalf("got pairs %+v, want exactly the pair with the contact on file", pairs)
	}
}

func TestEmploymentEdgeAtAnotherEmployerFilesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	onFile := mustContact(t, e, "Marlene Wiesinger")
	employ(t, e, onFile, mustEmployer(t, e, "Acme Robotics GmbH"))
	twin := mustContact(t, e, "Marlene Wiesingr")

	employ(t, e, twin, mustEmployer(t, e, "Globex Logistik AG"))

	if pairs := openCandidatePairs(ctx, t, e, "contact", twin.UUID); len(pairs) != 0 {
		t.Fatalf("got pairs %+v, want none for a different employer", pairs)
	}
}

func TestEmploymentEdgeForADifferentNameAtTheSameEmployerFilesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	acme := mustEmployer(t, e, "Acme Robotics GmbH")
	employ(t, e, mustContact(t, e, "Marlene Wiesinger"), acme)
	colleague := mustContact(t, e, "Tobias Hergenhahn")

	employ(t, e, colleague, acme)

	if pairs := openCandidatePairs(ctx, t, e, "contact", colleague.UUID); len(pairs) != 0 {
		t.Fatalf("got pairs %+v, want none for a colleague with another name", pairs)
	}
}

// Making an existing employment the current primary is the other way a contact
// gets its employer.
func TestPromotingAnEmploymentToCurrentPrimaryFilesTheTwin(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	acme := mustEmployer(t, e, "Acme Robotics GmbH")
	onFile := mustContact(t, e, "Marlene Wiesinger")
	employ(t, e, onFile, acme)
	twin := mustContact(t, e, "Marlene Wiesingr")
	no := false
	edge, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: employmentKind, ContactID: &twin, CompanyID: &acme, Source: "manual", IsCurrentPrimary: &no,
	})
	if err != nil {
		t.Fatalf("employ without primary: %v", err)
	}
	if pairs := openCandidatePairs(ctx, t, e, "contact", twin.UUID); len(pairs) != 0 {
		t.Fatalf("got %d pairs before the promotion, want 0", len(pairs))
	}

	yes := true
	if _, err := e.store.UpdateRelationship(e.asEditor(), ids.UUID(edge.ID), UpdateRelationshipInput{IsCurrentPrimary: &yes}); err != nil {
		t.Fatalf("promote: %v", err)
	}

	if pairs := openCandidatePairs(ctx, t, e, "contact", twin.UUID); len(pairs) != 1 {
		t.Fatalf("got pairs %+v, want the pair with the contact on file", pairs)
	}
}

func TestEmploymentEdgeNotesOnlyThePairItFiled(t *testing.T) {
	e := setupDedupe(t)
	acme := mustEmployer(t, e, "Acme Robotics GmbH")
	onFile := mustContact(t, e, "Marlene Wiesinger")
	employ(t, e, onFile, acme)
	twin := mustContact(t, e, "Marlene Wiesingr")
	ctx, sink := filedpairs.Collect(e.as())
	company := acme

	if _, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: employmentKind, ContactID: &twin, CompanyID: &company, Source: "manual",
	}); err != nil {
		t.Fatalf("employ: %v", err)
	}

	pairs, err := e.store.OpenCandidatesNaming(e.as(), entityContact, twin.UUID, nil)
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	noted := sink.IDs()
	if len(pairs) != 1 || len(noted) != 1 || noted[0] != pairs[0].ID {
		t.Fatalf("noted %v, queue has %+v, want the one pair the edge filed", noted, pairs)
	}
}

// Two similar-name contacts at different employers are no pair until a company
// merge puts them at one employer.
func TestMergingCompaniesFilesTheTwinsItBringsTogether(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor := mustEmployer(t, e, "Acme Robotics GmbH")
	retired := mustEmployer(t, e, "Acme Robotik GmbH")
	onFile := mustContact(t, e, "Marlene Wiesinger")
	employ(t, e, onFile, survivor)
	twin := mustContact(t, e, "Marlene Wiesingr")
	employ(t, e, twin, retired)
	if pairs := openCandidatePairs(ctx, t, e, "contact", twin.UUID); len(pairs) != 0 {
		t.Fatalf("got %d pairs before the merge, want 0", len(pairs))
	}

	if _, err := e.store.MergeCompany(ctx, retired, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	pairs := openCandidatePairs(ctx, t, e, "contact", twin.UUID)
	if len(pairs) != 1 || pairs[0].OtherID != onFile.String() {
		t.Fatalf("got pairs %+v, want exactly the pair with the contact on file", pairs)
	}
}

// Duplicate scoring reads only the current primary employer, so an ended or a
// non-primary employment moved by a merge files nothing.
func TestMergingCompaniesDoesNotRecheckContactsWhoseCurrentEmployerStays(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor := mustEmployer(t, e, "Acme Robotics GmbH")
	retired := mustEmployer(t, e, "Acme Robotik GmbH")
	employ(t, e, mustContact(t, e, "Marlene Wiesinger"), survivor)
	ended := mustContact(t, e, "Marlene Wiesingr")
	sideRole := mustContact(t, e, "Marlene Wiesinge")
	past := clocktest.Now(t).AddDate(-1, 0, 0)
	no := false
	for _, edge := range []CreateRelationshipInput{
		{Kind: employmentKind, ContactID: &ended, CompanyID: &retired, Source: "manual", EndedAt: &past},
		{Kind: employmentKind, ContactID: &sideRole, CompanyID: &retired, Source: "manual", IsCurrentPrimary: &no},
	} {
		if _, err := e.store.CreateRelationship(ctx, edge); err != nil {
			t.Fatalf("employ: %v", err)
		}
	}

	primary := mustContact(t, e, "Tobias Hergenhahn")
	employ(t, e, primary, retired)
	var listed map[ids.ContactID]ids.CompanyID
	if err := e.store.tx(ctx, func(tx pgx.Tx) (err error) {
		listed, err = primaryEmployersOf(ctx, tx, retired)
		return err
	}); err != nil {
		t.Fatalf("listing the primary employees: %v", err)
	}
	if len(listed) != 1 || listed[primary] != retired {
		t.Fatalf("primary employees of the retired company = %v, want only %s", listed, primary)
	}

	if _, err := e.store.MergeCompany(ctx, retired, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}

	for name, contact := range map[string]ids.ContactID{"an ended employment": ended, "a non-primary employment": sideRole} {
		if pairs := openCandidatePairs(ctx, t, e, "contact", contact.UUID); len(pairs) != 0 {
			t.Errorf("%s moved by the merge filed %+v, want no pair", name, pairs)
		}
	}
}
