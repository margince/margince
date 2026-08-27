// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Where the ADVISORY answer and the WRITE can come apart on a LINK.
//
// The advisory answer draws the button; the write honours it. A branch the write
// asks about the link and the page answers about the RECORD is a live "Put back"
// over a refusal, which is the whole defect this surface exists to remove — and
// the two cases here are the two shapes it took. Both need real rows: one is a
// predicate over the audit spine, the other an RBAC grid a double would decide
// by construction.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// advisoryAnswer is the answer the history READ would put on one entry — the
// page evaluator, over the same seam the write binds.
func advisoryAnswer(ctx context.Context, t *testing.T, e *integration.Env,
	entityType string, id, auditID ids.UUID,
) privacy.UndoabilityAnswer {
	t.Helper()
	answers, err := NewUndoabilityPage(restoreSeamFor(e)).ForRecord(ctx, entityType, id, []ids.UUID{auditID})
	if err != nil {
		t.Fatalf("judging the page holding entry %s: %v", auditID, err)
	}
	answer, judged := answers[auditID]
	if !judged {
		t.Fatalf("entry %s was not judged at all, so its button renders disabled with no reason", auditID)
	}
	return answer
}

// The page refuses a link whose END was erased, by the same name the write does.
//
// The boundary the page reads for a record row is keyed on the row's own
// (entity_type, entity_id), which for a link is ('relationship', edge_id) — an
// identity no write path in this tree records a scrub verb against. Asked of a
// link it answers "never erased" for every link there has ever been, so the page
// lit the button and the write refused behind_erasure_boundary on press.
//
// The link is deliberately LIVE, as in the write's own case: an Art. 17 erasure
// archives its subject's links on the way past, and an archived link is refused
// on its own terms — so a test that erased through the product would pass on that
// cascade whether the boundary was asked or not.
func TestThePageRefusesALinkBehindAnEndsErasureBoundaryByName(t *testing.T) {
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

	answer := advisoryAnswer(e.Admin(), t, e, "person", person, auditID)
	if answer.Undoable || answer.Reason != string(ReasonBehindErasureBoundary) {
		t.Errorf("the page answered %+v for a link behind its subject's erasure, want a refusal "+
			"naming %q — the write refuses it on press", answer, ReasonBehindErasureBoundary)
	}
}

// A seat that may CHANGE a link but not REMOVE one is refused the create entry.
//
// Reversing a create archives the link, and the archive asks the delete grant.
// The seeded rep role holds create/read/update on `relationship` and no delete,
// so a page that asked the update grant for every entry drew an enabled button
// over a 403 for the commonest seat in the product.
func TestThePageRefusesACreateEntryToASeatThatMayNotRemoveALink(t *testing.T) {
	e := integration.Setup(t)
	// Owned by the rep, whose seeded row scope is their own records: a person
	// they cannot write would be refused for a reason this case is not about.
	person := e.SeedPerson(t, "Ada Employed", &e.Rep1)
	org := e.SeedOrg(t, "Employer GmbH", &e.Rep1)
	edge := seedEmploymentEdge(t, e, person, org)
	auditID := latestAuditRowID(t, e, edgeEntityType, edge, "create")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	answer := advisoryAnswer(rep, t, e, "person", person, auditID)
	if answer.Undoable || answer.Reason != string(ReasonNotWritableByCaller) {
		t.Errorf("the page answered %+v to a seat holding relationship update and not delete, "+
			"want a refusal naming %q", answer, ReasonNotWritableByCaller)
	}

	// The control on the premise: this seat really cannot remove the link, so the
	// refusal above is the page agreeing with the write rather than a stricter
	// answer of its own.
	if _, err := restoreSeamFor(e).Restore(rep, "person", person, auditID,
		currentVersion(t, e, "person", person)); err == nil {
		t.Error("the reverse committed for a seat that holds no delete grant on a link")
	}
	if !edgeIsLive(t, e, edge) {
		t.Error("the refused reverse removed the link anyway")
	}
}
