// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package snapshot

import (
	"encoding/json"
	"strings"
	"testing"
)

const twoSites = `{
  "generated_from": "internal/compose/aicert/records",
  "rows": [
    {"task":"capture_classify","site":"classify","provider":"openai_compatible","model":"openai/gpt-oss-120b",
     "env_class":"eu_hosted","status":"current","band":"certified","scope":"full",
     "runs":21,"passed":21,"measured_scenarios":7,"pending_scenarios":0,"ran_at":"2026-09-01T12:00:00Z"},
    {"task":"capture_classify","site":"classify","provider":"openai_compatible","model":"openai/gpt-oss-120b",
     "env_class":"cloud_frontier","status":"stale","band":"supported_degraded","scope":"full",
     "runs":9,"passed":8,"measured_scenarios":3,"pending_scenarios":0,"ran_at":"2026-07-04T12:00:00Z"}
  ]
}`

// The environment class is part of the identity of a measurement, so a lookup
// under one posture must not find a row measured under another. Reading a
// cloud_frontier number as if it certified an eu_hosted binding is the one
// mistake this index exists to make impossible.
func TestForKeysOnTheEnvironmentClass(t *testing.T) {
	t.Parallel()

	snap, err := decode([]byte(twoSites))
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}

	row, found := snap.For("capture_classify", "classify", "openai_compatible", "openai/gpt-oss-120b", "eu_hosted")
	if !found {
		t.Fatal("the eu_hosted row was not found")
	}
	if row.Status != StatusCurrent || row.Runs != 21 {
		t.Errorf("found the wrong row: %+v", row)
	}
	if _, found := snap.For("capture_classify", "classify", "openai_compatible", "openai/gpt-oss-120b", "sovereign"); found {
		t.Error("a lookup under an unmeasured environment class found a row")
	}
}

// "We have never measured this" and "we have measured it elsewhere" are
// different answers, and only the second gives a reader something to go and
// read.
func TestMeasuredElsewhereSeesTheOtherPosture(t *testing.T) {
	t.Parallel()

	snap, err := decode([]byte(twoSites))
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if !snap.MeasuredElsewhere("capture_classify", "classify", "openai_compatible", "openai/gpt-oss-120b", "sovereign") {
		t.Error("the cloud_frontier measurement was not reported for a sovereign lookup")
	}
	if snap.MeasuredElsewhere("draft_reply", "reply", "openai_compatible", "openai/gpt-oss-120b", "eu_hosted") {
		t.Error("another site's rows were reported as this one's evidence")
	}

	// A row is not its own evidence: asked about the only posture measured, the
	// answer is no, or every certified row would claim corroboration from itself.
	single, err := decode([]byte(`{"generated_from":"x","rows":[
    {"task":"t","site":"s","provider":"p","model":"m","env_class":"eu_hosted","status":"current",
     "runs":3,"passed":3,"measured_scenarios":1,"pending_scenarios":0}]}`))
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if single.MeasuredElsewhere("t", "s", "p", "m", "eu_hosted") {
		t.Error("a row reported itself as a measurement from another posture")
	}
}

// Two rows on one key are two answers to one question. The snapshot is generated,
// so this is a generator bug reaching the binary — it must stop at load rather
// than resolve to whichever row happened to be indexed last.
func TestDecodeRefusesADuplicateKey(t *testing.T) {
	t.Parallel()

	const duplicated = `{"generated_from":"x","rows":[
    {"task":"t","site":"s","provider":"p","model":"m","env_class":"e","status":"current","runs":1,"passed":1,
     "measured_scenarios":1,"pending_scenarios":0},
    {"task":"t","site":"s","provider":"p","model":"m","env_class":"e","status":"stale","runs":2,"passed":0,
     "measured_scenarios":1,"pending_scenarios":0}]}`

	_, err := decode([]byte(duplicated))
	if err == nil {
		t.Fatal("two rows sharing a key were accepted")
	}
	if !strings.Contains(err.Error(), "two rows") || !strings.Contains(err.Error(), "make gen") {
		t.Errorf("the error must name the fault and the remedy, got: %v", err)
	}
}

// A field carrying the separator would let two distinct rows join to one key.
// Nothing in the vocabularies contains it, which is exactly why a violation is
// worth failing on rather than trusting.
func TestDecodeRefusesASeparatorInAField(t *testing.T) {
	t.Parallel()

	poisoned := `{"generated_from":"x","rows":[
    {"task":"t` + keySeparator + `x","site":"s","provider":"p","model":"m","env_class":"e","status":"absent",
     "runs":0,"passed":0,"measured_scenarios":0,"pending_scenarios":0}]}`

	if _, err := decode([]byte(poisoned)); err == nil {
		t.Fatal("a field containing the key separator was indexed")
	}
}

// The committed file must be loadable by the binary that embeds it — the whole
// point of generating it. A malformed commit fails here rather than at runtime
// on a customer's settings page.
func TestTheCommittedSnapshotLoads(t *testing.T) {
	t.Parallel()

	snap, err := Load()
	if err != nil {
		t.Fatalf("the committed snapshot does not load: %v", err)
	}
	for _, row := range snap.Rows {
		switch row.Status {
		case StatusCurrent, StatusPartial, StatusStale, StatusAbsent:
		default:
			t.Errorf("%s/%s carries status %q, which is not one of the four states", row.Task, row.Site, row.Status)
		}
		if row.Passed > row.Runs {
			t.Errorf("%s/%s passed %d of %d runs", row.Task, row.Site, row.Passed, row.Runs)
		}
	}
}

// Rows that did not arrive as a file go through the same indexing and the same
// duplicate refusal, which is why FromRows exists at all: a caller assembling
// the map itself would prove nothing about the lookup the binary performs.
func TestFromRowsIndexesAndRefusesLikeAFile(t *testing.T) {
	t.Parallel()

	row := Row{
		Task: "draft_reply", Site: "reply", Provider: "openai_compatible",
		Model: "openai/gpt-oss-120b", EnvClass: "eu_hosted",
		Status: StatusCurrent, Band: "certified", Runs: 9, Passed: 9, Measured: 3,
	}
	snap, err := FromRows([]Row{row})
	if err != nil {
		t.Fatalf("indexing: %v", err)
	}
	if _, found := snap.For(row.Task, row.Site, row.Provider, row.Model, row.EnvClass); !found {
		t.Error("a row given directly was not indexed")
	}
	if _, err := FromRows([]Row{row, row}); err == nil {
		t.Error("two rows sharing a key were accepted, which a file would have refused")
	}
}

// A Snapshot decoded straight into the struct has every row and no index, so it
// would answer "not measured" about a table holding the answer. Indexing lazily
// would have to swallow the duplicate refusal, so misuse is refused instead —
// the same call the package's own Verdict makes about an even run count.
func TestForRefusesASnapshotThatWasNeverIndexed(t *testing.T) {
	t.Parallel()

	var raw Snapshot
	if err := json.Unmarshal([]byte(twoSites), &raw); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(raw.Rows) == 0 {
		t.Fatal("the fixture decoded to no rows, so this proves nothing about the index")
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("an unindexed Snapshot answered a lookup instead of refusing")
		}
		if msg, ok := recovered.(string); !ok || !strings.Contains(msg, "never indexed") {
			t.Errorf("the panic must name the fault and the remedy, got: %v", recovered)
		}
	}()
	raw.For("capture_classify", "classify", "openai_compatible", "openai/gpt-oss-120b", "eu_hosted")
}
