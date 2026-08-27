// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The two branches of an edge reversal that are decided by state OUTSIDE the
// link: the erasure boundary of the records it joins, and whether this
// workspace's records live in an incumbent system.
//
// Both need real rows. The erasure boundary is a predicate over the audit spine
// and cannot be judged from a double, and the overlay answer is only interesting
// against a reversal that otherwise commits — a refusal that would have happened
// anyway proves nothing about the branch that was or was not asked.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// refusedBy asserts the reversal refused for exactly this reason.
func refusedBy(t *testing.T, err error, want Reason) {
	t.Helper()
	if err == nil {
		t.Fatalf("the reverse committed; want a refusal naming %q", want)
	}
	var refusal RefusedRestore
	if !errors.As(err, &refusal) {
		t.Fatalf("the reverse answered %v, want a refusal naming %q", err, want)
	}
	if refusal.Reason != want {
		t.Fatalf("the reverse refused with %q, want %q", refusal.Reason, want)
	}
}

// A link whose END was erased after the entry was written refuses BY NAME, and
// the link is untouched.
//
// The tombstone lands on the record whose history is being read — the one end
// the admission deliberately does not bound, because a row behind a boundary
// should be refused with a reason a person can act on rather than reported
// absent. What makes that division of labour honest is that the refusal is
// actually reachable, and for a link it is only reachable through the ENDS: the
// row's own identity is ('relationship', edge_id), and no write path in this tree
// records a scrub verb against `relationship`, so a boundary keyed on it answers
// "never erased" for every link there has ever been.
//
// The link is deliberately LIVE. An Art. 17 erasure archives its subject's links
// on the way past, and an archived link is refused on its own terms — so a test
// that erased through the product would pass on that cascade whether this
// boundary was asked or not.
func TestReversingALinkBehindAnEndsErasureBoundaryRefusesByName(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Selma Subject", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	edge := seedEmploymentEdge(t, e, person, org)
	auditID := latestAuditRowID(t, e, edgeEntityType, edge, "create")

	e.SeedScrubTombstone(t, "person", person, time.Now().Add(time.Hour).UTC())
	if !edgeIsLive(t, e, edge) {
		t.Fatal("the link is already archived, so the refusal below could come from that " +
			"and say nothing about the erasure boundary")
	}

	_, err := restoreSeamFor(e).Restore(e.Admin(), "person", person, auditID,
		currentVersion(t, e, "person", person))
	refusedBy(t, err, ReasonBehindErasureBoundary)
	if !edgeIsLive(t, e, edge) {
		t.Error("the refused reverse removed the link anyway")
	}
}

// The SAME boundary read from the other end. An employment image holds the role
// and the dates of both records, so an erasure of either end bounds it — and the
// endpoint columns are searched from one slice, so neither end can be the one
// somebody remembered.
func TestReversingALinkBehindTheOtherEndsErasureBoundaryRefusesByName(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Erased Holdings GmbH", nil)
	edge := seedEmploymentEdge(t, e, person, org)
	auditID := latestAuditRowID(t, e, edgeEntityType, edge, "create")

	// On the ORGANIZATION, read from the ORGANIZATION: the anchor's own boundary,
	// which the admission leaves to the evaluator, on the end whose column is not
	// the first in the slice.
	e.SeedScrubTombstone(t, "organization", org, time.Now().Add(time.Hour).UTC())

	_, err := restoreSeamFor(e).Restore(e.Admin(), "organization", org, auditID,
		currentVersion(t, e, "organization", org))
	refusedBy(t, err, ReasonBehindErasureBoundary)
	if !edgeIsLive(t, e, edge) {
		t.Error("the refused reverse removed the link anyway")
	}
}

// A tombstone OLDER than the entry does not bound it. The boundary is a moment,
// not a flag: a record scrubbed and then linked again has a link nobody erased,
// and refusing it would make every such link permanently un-reversible.
func TestALinkWrittenAfterAnEndsErasureIsStillReversible(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	e.SeedScrubTombstone(t, "person", person, time.Now().Add(-time.Hour).UTC())

	edge := seedEmploymentEdge(t, e, person, org)
	auditID := latestAuditRowID(t, e, edgeEntityType, edge, "create")
	if _, err := restoreSeamFor(e).Restore(e.Admin(), "person", person, auditID,
		currentVersion(t, e, "person", person)); err != nil {
		t.Fatalf("a link made after the erasure: %v", err)
	}
	if edgeIsLive(t, e, edge) {
		t.Error("the reverse answered success and left the link live")
	}
}

// A link in an OVERLAY-GOVERNED workspace still reverses, and the record path in
// the same workspace still refuses. One route, two answers, decided on purpose.
//
// The record refusal exists because an overlay workspace's records live in the
// incumbent: putting one back is a write-back that records its own verb and its
// own evidence, so nothing would read as undone and the change would already
// have happened in two systems. A LINK has no incumbent counterpart —
// `relationship` is not in the overlay mirror's entity set and the overlay
// provider declares no write verb for it — so the local row is not a copy of
// anything and the local write is the whole write. Refusing it would strand a
// link nobody can reverse in a workspace where nothing else could have changed
// it either.
//
// The pair is asserted together because the interesting failure is the two
// branches converging: a refusal added to the edge path, or the record's
// quietly dropped.
func TestALinkInAnOverlayGovernedWorkspaceIsStillReversible(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	edge := seedEmploymentEdge(t, e, person, org)
	title := "COO"
	if _, err := e.People.UpdatePerson(e.Admin(), ids.From[ids.PersonKind](person),
		people.UpdatePersonInput{Title: &title, Source: "manual"}); err != nil {
		t.Fatalf("change a field through the real writer: %v", err)
	}

	// The port rather than a connected incumbent: what is under test is which
	// branch asks it, and a real overlay workspace would also change what the
	// update path does with the answer.
	seam := restoreSeamFor(e)
	seam.evaluator.ExternallyGoverned = func(context.Context) (bool, error) { return true, nil }

	recordEntry := latestAuditRowID(t, e, "person", person, "update")
	_, err := seam.Restore(e.Admin(), "person", person, recordEntry,
		currentVersion(t, e, "person", person))
	refusedBy(t, err, ReasonNotRestorableByThisPath)

	edgeEntry := latestAuditRowID(t, e, edgeEntityType, edge, "create")
	if _, err := seam.Restore(e.Admin(), "person", person, edgeEntry,
		currentVersion(t, e, "person", person)); err != nil {
		t.Fatalf("reversing a link in an overlay-governed workspace: %v", err)
	}
	if edgeIsLive(t, e, edge) {
		t.Error("the reverse answered success and left the link live")
	}
}

// A reversal that overwrites a role does not slip past the boundary either. The
// create case above removes the link; this one WRITES a pre-erasure value back,
// which is the outcome the boundary exists to prevent.
func TestReplayingALinksPreErasureImageIsRefusedByName(t *testing.T) {
	e := integration.Setup(t)
	person := e.SeedPerson(t, "Selma Subject", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	edge := seedEmploymentEdge(t, e, person, org)
	changed := "coo"
	if _, err := e.People.UpdateRelationship(e.Admin(), edge,
		people.UpdateRelationshipInput{Role: &changed}); err != nil {
		t.Fatalf("change the link through the real writer: %v", err)
	}
	auditID := latestAuditRowID(t, e, edgeEntityType, edge, "update")

	e.SeedScrubTombstone(t, "person", person, time.Now().Add(time.Hour).UTC())

	_, err := restoreSeamFor(e).Restore(e.Admin(), "person", person, auditID,
		currentVersion(t, e, "person", person))
	refusedBy(t, err, ReasonBehindErasureBoundary)
	if role := roleOf(t, e, edge); role != changed {
		t.Errorf("the link's role is %q; the refused reverse wrote the pre-erasure value back", role)
	}
}

// roleOf reads the link's current role, which is what a replay would move.
func roleOf(t *testing.T, e *integration.Env, edge ids.UUID) string {
	t.Helper()
	admin := e.Admin()
	var role *string
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(admin, `SELECT role FROM relationship WHERE id = $1`, edge).Scan(&role)
	}); err != nil {
		t.Fatalf("read link %s's role: %v", edge, err)
	}
	if role == nil {
		return ""
	}
	return *role
}
