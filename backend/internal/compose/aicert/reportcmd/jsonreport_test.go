// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aicert/snapshot"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

func certifiedRow(task, variant, model, env, band string, runs, passed int) readinessRow {
	return readinessRow{
		site:      aitasks.Site{Task: ai.Task(task), Variant: variant},
		scope:     "full",
		certified: true,
		measured:  3,
		record: aicert.Record{
			Task: task, Provider: "openai_compatible", ServedModel: model,
			EnvClass: env, CertifiedScope: "full", RanAt: "2026-09-01T00:00:00Z",
		},
		tally: aicert.SiteTally{Verdict: band, Runs: runs, Passed: passed},
	}
}

// The snapshot is committed and diffed by a drift gate, so the same rows must
// encode to the same bytes however they arrived. An unstable order would show as
// a change on every regeneration and train a reader to ignore the diff.
func TestRenderJSONSortsByKey(t *testing.T) {
	t.Parallel()

	rows := []readinessRow{
		certifiedRow("draft_reply", "reply", "z-model", "eu_hosted", "certified", 9, 9),
		certifiedRow("capture_classify", "classify", "a-model", "eu_hosted", "certified", 9, 9),
		certifiedRow("draft_reply", "reply", "a-model", "eu_hosted", "certified", 9, 9),
	}
	encoded, err := renderJSON("internal/compose/aicert/records", rows)
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}

	var got snapshot.Snapshot
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("the rendered snapshot does not decode: %v", err)
	}
	want := []string{"capture_classify/a-model", "draft_reply/a-model", "draft_reply/z-model"}
	for i, row := range got.Rows {
		if key := row.Task + "/" + row.Model; key != want[i] {
			t.Errorf("row %d is %s, want %s", i, key, want[i])
		}
	}
	if !strings.HasSuffix(string(encoded), "}\n") {
		t.Error("the file must end in a newline, or every diff shows a no-newline marker")
	}
}

// An absent site is the finding the report exists to surface. It must survive
// serialisation as absence — no band, no timestamp, no binding — because a
// zeroed band word would read as a measurement that returned nothing.
func TestRenderJSONKeepsAbsenceEmptyRatherThanZeroed(t *testing.T) {
	t.Parallel()

	rows := []readinessRow{{
		site:  aitasks.Site{Task: ai.TaskBriefRanking, Variant: "ranking"},
		scope: "full",
	}}
	encoded, err := renderJSON("records", rows)
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	var got snapshot.Snapshot
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	row := got.Rows[0]
	if row.Status != snapshot.StatusAbsent {
		t.Fatalf("status = %q, want %q", row.Status, snapshot.StatusAbsent)
	}
	if row.Band != "" || row.RanAt != "" || row.Model != "" || row.Provider != "" {
		t.Errorf("an absent row carries a measurement: %+v", row)
	}
	if row.Runs != 0 || row.Passed != 0 {
		t.Errorf("an absent row carries run counts: %+v", row)
	}
	// The scope survives: it is what a run COULD claim, and it is the only thing
	// an uncertified site can still say about itself.
	if row.Scope != "full" {
		t.Errorf("scope = %q, want the scope the site could claim", row.Scope)
	}
}

// The text report deliberately prints one site once per binding that measured
// it. The snapshot's key admits one, so two rows sharing it are a generation
// fault — it must stop rather than index whichever came last.
func TestRenderJSONRefusesTwoRowsOnOneKey(t *testing.T) {
	t.Parallel()

	same := certifiedRow("capture_classify", "classify", "gpt-oss-120b", "eu_hosted", "certified", 9, 9)
	_, err := renderJSON("records", []readinessRow{same, same})
	if err == nil {
		t.Fatal("a duplicated key was rendered")
	}
	if !strings.Contains(err.Error(), "capture_classify/classify") {
		t.Errorf("the error must name the colliding key, got: %v", err)
	}
}

// Whatever renderJSON writes, the leaf package must be able to load — they are
// the two halves of one file format, and a change to either that the other has
// not seen breaks the product rather than a test.
func TestTheRenderedSnapshotLoadsInTheLeafPackage(t *testing.T) {
	t.Parallel()

	rows := []readinessRow{
		certifiedRow("capture_classify", "classify", "gpt-oss-120b", "eu_hosted", "certified", 21, 21),
		certifiedRow("draft_reply", "reply", "gpt-oss-120b", "cloud_frontier", "not_supported", 24, 23),
	}
	encoded, err := renderJSON("records", rows)
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	var got snapshot.Snapshot
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(got.Rows))
	}
	// A high pass rate under a failing band is the case the card must not smooth
	// over: the verdict folds to the worst scenario, so 23 of 24 can still be
	// not_supported.
	for _, row := range got.Rows {
		if row.Task == "draft_reply" && row.Band != "not_supported" {
			t.Errorf("draft_reply band = %q, want the record's own verdict", row.Band)
		}
	}
}
