// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
)

// The committed ladder verdict table is what the committed records give.
func TestTheTierCertTableIsCurrent(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the census: %v", err)
	}
	corpus, err := LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	records, err := LoadRecords("records")
	if err != nil {
		t.Fatalf("loading the records: %v", err)
	}
	rows, err := TierCertTable(context.Background(), corpus, census, records)
	if err != nil {
		t.Fatalf("building the table: %v", err)
	}
	want, err := RenderTierCertTable(rows)
	if err != nil {
		t.Fatalf("rendering the table: %v", err)
	}
	committed, err := os.ReadFile(filepath.Join("..", "..", "modules", "ai", "tiercert_gen.go"))
	if err != nil {
		t.Fatalf("reading the committed table: %v", err)
	}
	if !bytes.Equal(committed, want) {
		t.Fatal("the ladder verdict table is not what the committed records give — run `make gen`")
	}
}

// One binding's verdict is the worst of its records, within a site and across
// the sites of the task.
func TestATierVerdictIsTheWorstOfItsRecordsAndSites(t *testing.T) {
	record := func(verdict string) Record {
		return Record{Task: "draft_reply", Provider: "p", ServedModel: "m", EnvClass: "cloud_frontier", Verdict: verdict}
	}
	row := func(site, verdict string) ReadinessRow {
		return ReadinessRow{Site: aitasks.Site{Variant: site}, Certified: true, Record: record(verdict)}
	}
	for name, tc := range map[string]struct {
		scenarios []Scenario
		rows      []ReadinessRow
		want      string
	}{
		"two records on one site": {
			[]Scenario{{Site: "a"}},
			[]ReadinessRow{row("a", VerdictCertified), row("a", VerdictNotSupported)},
			VerdictNotSupported,
		},
		"the same two records in the other order": {
			[]Scenario{{Site: "a"}},
			[]ReadinessRow{row("a", VerdictNotSupported), row("a", VerdictCertified)},
			VerdictNotSupported,
		},
		"a pass on one site and a degraded run on another": {
			[]Scenario{{Site: "a"}, {Site: "b"}},
			[]ReadinessRow{row("a", VerdictCertified), row("b", VerdictSupportedDegraded)},
			VerdictSupportedDegraded,
		},
		"a site no record measured": {
			[]Scenario{{Site: "a"}, {Site: "b"}},
			[]ReadinessRow{row("a", VerdictCertified)},
			VerdictNotSupported,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := taskTierCerts(tc.scenarios, tc.rows)
			if len(got) != 1 || got[0].Cert.Verdict != tc.want {
				t.Fatalf("got %+v, want one row with verdict %q", got, tc.want)
			}
		})
	}
}
