// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The overnight agent writing its findings onto a brief, over the real handler
// stack and real Postgres.
//
// Every refusal here is paired with an ADMIT case in the same file. A refusal
// test alone proves nothing: an endpoint that refused everybody would pass all
// of them, and this suite's whole subject is an authority that must say yes to
// exactly one caller and no to every other.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// annotatedBrief adds the annotation fields to what the e2e suite already reads.
type annotatedBrief struct {
	ID          string                  `json:"id"`
	Narrative   *string                 `json:"narrative"`
	AnnotatedAt *string                 `json:"annotated_at"`
	Items       []annotatedBriefItemRow `json:"items"`
}

type annotatedBriefItemRow struct {
	ID          string   `json:"id"`
	DealID      string   `json:"deal_id"`
	EvidenceIds []string `json:"evidence_ids"`
	Finding     *string  `json:"finding"`
}

// seedRunWithItems generates a brief the annotation tests can write onto.
func seedRunWithItems(t *testing.T, e *apptest.AppEnv) annotatedBrief {
	t.Helper()
	stages := apptest.DiscoverSeededPipeline(t, e)
	createDealClosingThisWeek(t, e, stages, "Closing this week")
	createDealClosingThisWeek(t, e, stages, "Also closing")

	var run annotatedBrief
	if status := e.Call(t, "POST", "/v1/brief", nil, nil, &run); status != http.StatusCreated {
		t.Fatalf("POST /v1/brief = %d, want 201", status)
	}
	if len(run.Items) < 2 {
		t.Fatalf("seeded run has %d item(s), want at least 2", len(run.Items))
	}
	// Nothing has annotated it yet, and both fields say so.
	if run.Narrative != nil || run.AnnotatedAt != nil {
		t.Fatalf("a freshly generated run already carries annotation: %+v", run)
	}
	return run
}

func readBriefRun(t *testing.T, e *apptest.AppEnv) annotatedBrief {
	t.Helper()
	var run annotatedBrief
	if status := e.Call(t, "GET", "/v1/brief", nil, nil, &run); status != http.StatusOK {
		t.Fatalf("GET /v1/brief = %d, want 200", status)
	}
	return run
}

// THE ADMIT CASE. Without it every refusal below could be satisfied by an
// endpoint that refuses everyone.
func TestAnAnnotationWithVerifiableCitationsIsWritten(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	run := seedRunWithItems(t, e)
	item := run.Items[0]

	status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
		"narrative": "Two replies overnight, one deal went quiet.",
		"items": []AnyMap{{
			"item_id":        item.ID,
			"finding":        "He asked about the delivery date and you usually answer within a day.",
			"cited_evidence": item.EvidenceIds[:1],
		}},
	}, nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("annotate = %d, want 204", status)
	}

	after := readBriefRun(t, e)
	if after.Narrative == nil || *after.Narrative == "" {
		t.Fatal("the run carries no narrative after a successful annotation")
	}
	if after.AnnotatedAt == nil {
		t.Fatal("the run carries no annotated_at stamp, so the screen cannot tell a quiet night from a pass that never ran")
	}
	var found *string
	for _, row := range after.Items {
		if row.ID == item.ID {
			found = row.Finding
		}
	}
	if found == nil || *found == "" {
		t.Error("the annotated item carries no finding")
	}
}

// A pass that ran and had nothing to say is NOT the same as no pass. The stamp
// is what carries that difference, and it is the whole reason two columns exist
// rather than one.
func TestAQuietNightIsStampedEvenWithNothingToSay(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	seedRunWithItems(t, e)

	if status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
		"narrative": "",
		"items":     []AnyMap{},
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("annotate with an empty narrative = %d, want 204", status)
	}

	after := readBriefRun(t, e)
	if after.Narrative != nil {
		t.Errorf("an empty narrative was stored as %q, want null", *after.Narrative)
	}
	if after.AnnotatedAt == nil {
		t.Error("a pass that ran and found nothing left no stamp — indistinguishable from one that never ran")
	}
}

// THE REFUSAL THE PLAN NAMES: a model supplies uuids as text, and a uuid that
// parses is not a uuid that means anything.
func TestACitationOutsideTheItemsOwnEvidenceRefusesTheWholeWrite(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	run := seedRunWithItems(t, e)
	first, second := run.Items[0], run.Items[1]

	// A real evidence id, belonging to a DIFFERENT item in the same run. This
	// is the shape a plausible-looking hallucination takes: it is not invented,
	// it is simply not evidence for the claim it is attached to.
	borrowed := second.EvidenceIds[0]
	status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
		"narrative": "Something happened.",
		"items": []AnyMap{{
			"item_id":        first.ID,
			"finding":        "This claims to rest on a row that belongs to another deal.",
			"cited_evidence": []string{borrowed},
		}},
	}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("annotating with a borrowed citation = %d, want 422", status)
	}

	// NOTHING was written — not the narrative, not the other item. A partial
	// write would leave prose on the screen that the refusal was supposed to
	// have prevented.
	after := readBriefRun(t, e)
	if after.Narrative != nil || after.AnnotatedAt != nil {
		t.Errorf("a refused annotation still wrote to the run: %+v", after)
	}
}

func TestAnItemFromAnotherRunCannotBeAnnotated(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	seedRunWithItems(t, e)

	// A well-formed uuid that names no item in this run.
	status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
		"items": []AnyMap{{
			"item_id":        "01a04000-0000-7000-8000-000000000000",
			"finding":        "A finding about a queue entry that is not in this brief.",
			"cited_evidence": []string{"01a04000-0000-7000-8000-000000000001"},
		}},
	}, nil, nil)
	// Not-found, not forbidden: existence-hiding, like every row-scope miss.
	if status != http.StatusNotFound {
		t.Fatalf("annotating a foreign item = %d, want 404", status)
	}
}

func TestAnnotatingWithNoRunTodayIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	// Deliberately no POST /v1/brief: this rep has no run today.
	status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
		"narrative": "A sentence about a morning that was never assembled.",
		"items":     []AnyMap{},
	}, nil, nil)
	// Inventing a run here would produce a brief carrying prose with no ranking
	// behind it.
	if status != http.StatusNotFound {
		t.Fatalf("annotating with no run = %d, want 404", status)
	}
}

func TestASecondPassReplacesTheFirstRatherThanAppending(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	run := seedRunWithItems(t, e)
	item := run.Items[0]

	for _, narrative := range []string{"First reading of the night.", "Corrected reading."} {
		if status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
			"narrative": narrative,
			"items": []AnyMap{{
				"item_id":        item.ID,
				"finding":        narrative + " (finding)",
				"cited_evidence": item.EvidenceIds[:1],
			}},
		}, nil, nil); status != http.StatusNoContent {
			t.Fatalf("annotate %q = %d, want 204", narrative, status)
		}
	}

	after := readBriefRun(t, e)
	// A pass that ran twice has one answer, not two — the rep must not read
	// last night's finding stacked under tonight's.
	if after.Narrative == nil || *after.Narrative != "Corrected reading." {
		t.Errorf("the run carries %v, want only the corrected reading", after.Narrative)
	}
}

func TestProseBeyondTheCeilingIsRefusedWithSomethingActionable(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	seedRunWithItems(t, e)

	long := make([]byte, 601)
	for i := range long {
		long[i] = 'x'
	}
	status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
		"narrative": string(long),
		"items":     []AnyMap{},
	}, nil, nil)
	// 422 rather than a driver error surfacing as a failed run: an agent can
	// read this and shorten, which is the whole point of refusing in Go as well
	// as in the CHECK constraint.
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("over-long narrative = %d, want 422", status)
	}
}

// A finding that cites nothing is not "a claim with no sources" — it is the
// verification being skipped, and it is the easy path: omit the field and the
// prose reaches the rep's screen unchecked, under the same agent tag a grounded
// finding carries.
func TestAFindingThatCitesNothingIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	run := seedRunWithItems(t, e)

	status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
		"items": []AnyMap{{
			"item_id": run.Items[0].ID,
			"finding": "He is going to sign this week.",
		}},
	}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a finding with no citations = %d, want 422", status)
	}

	after := readBriefRun(t, e)
	if after.AnnotatedAt != nil {
		t.Error("a refused annotation still stamped the run")
	}
}

// A pass REPLACES. A finding the new pass did not restate must not survive
// under the new pass's stamp: the rep would read last night's explanation of a
// reply that has since been answered, dated tonight.
func TestASecondPassClearsFindingsItDoesNotRestate(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	run := seedRunWithItems(t, e)
	first, second := run.Items[0], run.Items[1]

	// Night one annotates both items.
	if status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
		"narrative": "Two things moved.",
		"items": []AnyMap{
			{"item_id": first.ID, "finding": "First deal moved.", "cited_evidence": first.EvidenceIds[:1]},
			{"item_id": second.ID, "finding": "Second deal moved.", "cited_evidence": second.EvidenceIds[:1]},
		},
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("first pass = %d, want 204", status)
	}

	// Night two has something to say about the first deal only.
	if status := e.Call(t, "PUT", "/v1/brief/annotations", AnyMap{
		"narrative": "One thing moved.",
		"items": []AnyMap{
			{"item_id": first.ID, "finding": "First deal moved again.", "cited_evidence": first.EvidenceIds[:1]},
		},
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("second pass = %d, want 204", status)
	}

	after := readBriefRun(t, e)
	for _, row := range after.Items {
		switch row.ID {
		case first.ID:
			if row.Finding == nil || *row.Finding != "First deal moved again." {
				t.Errorf("the restated item carries %v, want the second pass's finding", row.Finding)
			}
		case second.ID:
			if row.Finding != nil {
				t.Errorf("an item the second pass did not mention still carries %q — "+
					"last night's explanation, dated tonight", *row.Finding)
			}
		}
	}
}
