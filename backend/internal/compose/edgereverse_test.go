// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The refusals an edge reversal decides before it reads the trail, and the one
// pure judgement it makes about an image.
//
// A nil transaction is the assertion in each case: a branch that reached the
// trail would panic rather than pass on a value nobody supplied. The two that DO
// read the trail — supersession against the edge, and the erasure boundary — are
// proved in the integration lane against real rows.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/overlay"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// edgeRow is one audited change to a link, as the reversal path receives it.
func edgeRow(action, before, after string) AuditRow {
	return AuditRow{
		ID:         ids.NewV7(),
		EntityType: edgeEntityType,
		EntityID:   ids.NewV7(),
		Action:     action,
		Before:     json.RawMessage(before),
		After:      json.RawMessage(after),
	}
}

// employmentEdge is the live employment every case below starts from: the kind
// whose anchor is the PERSON, which is what makes the two ends asymmetric.
func employmentEdge() people.EdgeFacts {
	return people.EdgeFacts{Kind: "employment", Version: 3, Anchor: "person", AnchorID: ids.NewV7()}
}

// edgeEvaluator wires the edge reader to a fixed reading, which is what the
// binding path does too: the version the write pins has to be the version the
// decision judged.
func edgeEvaluator(facts people.EdgeFacts) Evaluator {
	return Evaluator{
		EdgeFacts: func(context.Context, pgx.Tx, ids.UUID) (people.EdgeFacts, error) {
			return facts, nil
		},
	}
}

func judgeEdge(t *testing.T, e Evaluator, row AuditRow) Undoability {
	t.Helper()
	var noTx pgx.Tx
	answer, err := e.Evaluate(context.Background(), noTx, row, Advisory)
	if err != nil {
		t.Fatalf("evaluate the edge entry: %v", err)
	}
	return answer
}

// Putting a REMOVED link back is an un-archive, and no write path here performs
// one. The refusal names itself so the surface can say what the reader CAN do —
// make the link again from the record's own screen — where a generic reason
// would be a dead end.
func TestReversingAnUnlinkRefusesByName(t *testing.T) {
	row := edgeRow(edgeActionArchive, `null`, `{"kind":"employment","role":"cto"}`)
	answer := judgeEdge(t, edgeEvaluator(employmentEdge()), row)
	if answer.Reason != ReasonEdgeRelinkUnsupported {
		t.Errorf("an unlink: reason = %q, want %q", answer.Reason, ReasonEdgeRelinkUnsupported)
	}
}

// A project's company is refused BY NAME whatever the verb, including the unlink
// the branch above would otherwise have answered. It needs write authority over
// the project ROW rather than the object grant, and a project must keep at least
// one company — so a generic reverse of one is a side door around both.
func TestReversingAProjectCompanyRefusesByNamingTheKind(t *testing.T) {
	facts := employmentEdge()
	facts.Kind = people.ProjectCompanyKind
	for _, action := range []string{"create", "update", edgeActionArchive} {
		answer := judgeEdge(t, edgeEvaluator(facts), edgeRow(action, `null`, `{"kind":"project_company"}`))
		if answer.Reason != ReasonNotRestorableByThisPath || answer.Detail != people.ProjectCompanyKind {
			t.Errorf("%s on a project's company: %q/%q, want %q naming the kind",
				action, answer.Reason, answer.Detail, ReasonNotRestorableByThisPath)
		}
	}
}

// Making a link and changing one both have an inverse. Anything else does not,
// and says which verb it was rather than refusing anonymously.
func TestOnlyMakingAndChangingALinkHaveAnInverse(t *testing.T) {
	for _, action := range []string{"create", "update"} {
		if !reversibleEdgeAction(action) {
			t.Errorf("%s has an inverse and was rejected", action)
		}
	}
	answer := judgeEdge(t, edgeEvaluator(employmentEdge()), edgeRow("merge", `null`, `{}`))
	if answer.Reason != ReasonNotAReplayableVerb || answer.Detail != "merge" {
		t.Errorf("merge: %q/%q, want %q naming the verb", answer.Reason, answer.Detail, ReasonNotAReplayableVerb)
	}
}

// The link this entry made has since been removed, so putting it back is the
// un-archive again. The state is named rather than the entry being offered.
func TestReversingTheCreationOfARemovedLinkSaysTheLinkIsGone(t *testing.T) {
	facts := employmentEdge()
	facts.Archived = true
	answer := judgeEdge(t, edgeEvaluator(facts), edgeRow("create", `null`, `{"kind":"employment"}`))
	if answer.Reason != ReasonRecordArchived {
		t.Errorf("an archived link: reason = %q, want %q", answer.Reason, ReasonRecordArchived)
	}
}

// An edge's write authority is its ANCHOR's, and the two ends are NOT
// symmetric: an employment anchors the person, so a seat holding
// organization-write and not person-write is refused the button on the COMPANY's
// page — where the record they are reading is one they may change.
//
// The refusal itself is not surfaced. It separates "not yours" from "does not
// exist", which is the distinction the row-scope gate keeps hidden.
func TestAnEdgeIsRefusedWhenItsAnchorIsNotTheCallersToWrite(t *testing.T) {
	e := edgeEvaluator(employmentEdge())
	e.EdgeWritable = func(context.Context, pgx.Tx, people.EdgeFacts) error {
		return apperrors.ErrPermissionDenied
	}
	answer := judgeEdge(t, e, edgeRow("create", `null`, `{"kind":"employment","role":"cto"}`))
	if answer.Reason != ReasonNotWritableByCaller {
		t.Errorf("no write authority on the anchor: reason = %q, want %q",
			answer.Reason, ReasonNotWritableByCaller)
	}
	if answer.Detail != "" {
		t.Errorf("the refusal carries %q; the row-scope answer must not reach the caller", answer.Detail)
	}
}

// A caller who may not see the link at all is answered about their authority and
// never with a fault. The row reached them through the history read's own gates,
// so its existence is not what is hidden here.
func TestAnEdgeTheCallerCannotReadIsAnswered(t *testing.T) {
	e := Evaluator{EdgeFacts: func(context.Context, pgx.Tx, ids.UUID) (people.EdgeFacts, error) {
		return people.EdgeFacts{}, apperrors.ErrNotFound
	}}
	answer := judgeEdge(t, e, edgeRow("create", `null`, `{"kind":"employment"}`))
	if answer.Reason != ReasonNotWritableByCaller {
		t.Errorf("an unreadable link: reason = %q, want %q", answer.Reason, ReasonNotWritableByCaller)
	}
}

// An edge patch coalesces every field against the column, so it cannot write a
// NULL: reversing an entry that FILLED a field in would report success and leave
// the value standing. The refusal names the fields rather than doing that.
func TestReversingAnEntryThatFilledALinkFieldInIsRefusedByName(t *testing.T) {
	row := edgeRow("update",
		`{"ended_at":null,"role":null}`,
		`{"ended_at":"2026-01-31T00:00:00Z","role":"coo"}`)
	answer := judgeEdge(t, edgeEvaluator(employmentEdge()), row)
	if answer.Reason != ReasonNullUnwritableByModule {
		t.Fatalf("reason = %q, want %q", answer.Reason, ReasonNullUnwritableByModule)
	}
	if answer.Detail != "ended_at, role" {
		t.Errorf("detail = %q, want both fields named and sorted", answer.Detail)
	}
}

// A field this entry MOVED between two values goes back the ordinary way. Only
// the null side is unwritable, and reading the pair too broadly would make every
// edge patch permanently un-undoable.
func TestReversingAnOrdinaryLinkChangeIsNotRefusedAsUnclearable(t *testing.T) {
	unclearable, err := edgeFieldsNoPatchCanClear(
		json.RawMessage(`{"role":"cto"}`), json.RawMessage(`{"role":"coo"}`),
	)
	if err != nil {
		t.Fatalf("judging the image: %v", err)
	}
	if len(unclearable) != 0 {
		t.Errorf("a value-to-value change reads as unclearable: %v", unclearable)
	}
}

// A create carries no before-image, and an entry that made a link is reversed by
// removing it rather than by replaying anything. Reading the absent image as a
// refusal would close that direction off entirely.
func TestTheAbsentBeforeImageOfACreateIsNotAnUnclearableField(t *testing.T) {
	unclearable, err := edgeFieldsNoPatchCanClear(nil, json.RawMessage(`{"kind":"employment","role":"cto"}`))
	if err != nil {
		t.Fatalf("judging the image: %v", err)
	}
	if len(unclearable) != 0 {
		t.Errorf("a create's absent image reads as unclearable: %v", unclearable)
	}
}

// The PREMISE under the edge path's overlay exemption, pinned so it cannot go
// quietly wrong.
//
// evaluateEdge does not ask ExternallyGoverned, and the reason is that a link has
// no incumbent counterpart to write back to: `relationship` has no overlay mirror
// projection, and the overlay provider declares no write verb for it. That is a
// fact about another package, so the exemption is only as sound as the fact — and
// the day the mirror learns to carry links, a reversal there would write the local
// half of a link the incumbent also holds, and the two would disagree silently.
//
// TestALinkInAnOverlayGovernedWorkspaceIsStillReversible holds the behaviour
// against real rows; this holds the reason for it. Both must be revisited
// together, which is why each names the other.
func TestNoOverlayWriteVerbServesALink(t *testing.T) {
	verbs := overlay.AllWriteVerbs()
	if len(verbs) == 0 {
		t.Fatal("no overlay write verbs to ask about; the premise would read as sound " +
			"because the corpus is empty")
	}
	for _, verb := range verbs {
		if overlay.SupportsWrite(verb, datasource.EntityRelationship) {
			t.Errorf("the overlay provider now serves %q for a link, so an overlay workspace "+
				"can hold one in its incumbent. evaluateEdge exempts the edge path from "+
				"ReasonNotRestorableByThisPath on the grounds that it cannot — a reversal "+
				"there would now write the local half alone and leave the two systems "+
				"disagreeing with nobody told", verb)
		}
	}
}
