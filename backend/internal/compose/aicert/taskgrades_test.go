// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The generated task grade table is reproduced from the committed trees. The
// grade Settings reads from it equals the report's for every preset's lead rung.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

var committedTaskGrades = filepath.Join("..", "..", "modules", "ai", "taskgrades_gen.go")

// gradeTrees is the committed corpus, records and presets, and the table they give.
type gradeTrees struct {
	census  *aitasks.Registry
	corpus  []Scenario
	records []Record
	rows    []TaskGradeRow
	presets map[string]ai.RoutingConfig
}

func loadGradeTrees(t *testing.T) gradeTrees {
	t.Helper()
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
	rows, err := TaskGrades(context.Background(), corpus, census, records)
	if err != nil {
		t.Fatalf("grading the records: %v", err)
	}
	presets := map[string]ai.RoutingConfig{}
	dir := filepath.Join("..", "..", "..", "..", "config", "presets")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("reading preset %s: %v", entry.Name(), err)
		}
		cfg, err := ai.ParsePreset(raw)
		if err != nil {
			t.Fatalf("preset %s: %v", entry.Name(), err)
		}
		presets[entry.Name()] = cfg
	}
	if len(presets) == 0 {
		t.Fatalf("no preset under %s, so the comparison below compares nothing", dir)
	}
	return gradeTrees{census: census, corpus: corpus, records: records, rows: rows, presets: presets}
}

// The committed table is what the committed corpus, census and records give.
// A re-run or a prompt edit that moves a grade fails here until `make gen`.
func TestTheTaskGradeTableIsCurrent(t *testing.T) {
	trees := loadGradeTrees(t)
	want, err := RenderTaskGrades(trees.rows)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(committedTaskGrades)
	if err != nil {
		t.Fatalf("reading %s: %v", committedTaskGrades, err)
	}
	if !bytes.Equal(committed, want) {
		t.Fatalf("%s is not what the committed records give — run `make gen`", committedTaskGrades)
	}
	completion := 0
	for _, rec := range trees.records {
		if rec.Kind != KindDecision {
			completion++
		}
	}
	// Every completion record measures the binding it names, so a row missing
	// here is a record the reconstruction lost, not one with nothing to say.
	if len(trees.rows) != completion {
		t.Errorf("the table grades %d records and the tree holds %d completion records", len(trees.rows), completion)
	}
}

// Settings grades a task's lead model by the generated table, and the report
// grades a preset's first rung by PresetRungs. Under every committed preset the
// two agree task by task. Otherwise an admin reads two grades for one setup.
func TestSettingsGradesEveryPresetLikeTheReport(t *testing.T) {
	trees := loadGradeTrees(t)
	compared := 0
	for file, cfg := range trees.presets {
		rungs, err := PresetRungs(context.Background(), cfg, trees.corpus, trees.census, trees.records)
		if err != nil {
			t.Fatalf("preset %s: %v", file, err)
		}
		routes := map[string]*ai.Certification{}
		for _, row := range ai.FeatureRoutes(cfg, cfg, ai.BandNormal) {
			if row.Certification != nil {
				routes[row.Task] = &ai.Certification{Grade: row.Certification.Grade, RecheckPending: row.Certification.RecheckPending}
			}
		}
		for _, rung := range rungs {
			got, shown := routes[string(rung.Task)]
			if !shown {
				t.Errorf("preset %s: the report grades %s and Settings shows no grade for it", file, rung.Task)
				continue
			}
			want := reportCertification(rung.FirstState, rung.FirstVerdict)
			if *got != want {
				t.Errorf("preset %s: %s on %s reads %+v in Settings and %s · %s in the report",
					file, rung.Task, rung.FirstModel, *got, rung.FirstState, rung.FirstVerdict)
			}
			compared++
		}
	}
	if compared == 0 {
		t.Fatal("no preset rung was compared, so this test proved nothing")
	}
}

// reportCertification is the report's state and verdict in Settings' words.
func reportCertification(state, verdict string) ai.Certification {
	if state == StatusAbsent {
		return ai.Certification{Grade: ai.CertGradeNotMeasured}
	}
	grades := map[string]string{
		VerdictCertified: ai.CertGradeReady, VerdictSupportedDegraded: ai.CertGradeWithCare, VerdictNotSupported: ai.CertGradeNotReliable,
	}
	return ai.Certification{Grade: grades[verdict], RecheckPending: state != StatusCurrent}
}
