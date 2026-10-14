// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func schemaEnum(t *testing.T, schema string) []string {
	t.Helper()
	s, ok := loadContract(t).Components.Schemas[schema]
	if !ok || s.Value == nil || len(s.Value.Enum) == 0 {
		t.Fatalf("crm.yaml declares no %s enum", schema)
	}
	var out []string
	for _, v := range s.Value.Enum {
		text, isText := v.(string)
		if !isText {
			t.Fatalf("%s value %v is not text", schema, v)
		}
		out = append(out, text)
	}
	slices.Sort(out)
	return out
}

// Compose holds this test because a module may not import the contract
// package. A code added on either side alone fails here.
func TestReasonCodesMatchTheContractEnum(t *testing.T) {
	var got []string
	for _, code := range migration.ReasonCodes() {
		got = append(got, string(code))
	}
	if want := schemaEnum(t, "ImportReasonCode"); !slices.Equal(got, want) {
		t.Fatalf("migration codes %v, contract codes %v", got, want)
	}
	phases := migration.Phases()
	slices.Sort(phases)
	if want := schemaEnum(t, "ImportRunPhase"); !slices.Equal(phases, want) {
		t.Fatalf("migration phases %v, contract phases %v", phases, want)
	}
}

// Every place that words a refusal sets the code with it, so a screen never
// meets a skip it can only show in English. The census reads the writers' own
// source: a literal that sets the sentence and not the code fails here.
func TestEveryReasonWriterSetsACode(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) < 100 {
		t.Fatalf("found %d source files (%v), want the whole package", len(files), err)
	}
	var literals, coded int
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			keys := literalKeys(lit)
			switch sel.Sel.Name {
			case "SkippedRow":
				literals++
				if keys["Reason"] && !keys["Code"] {
					t.Errorf("%s: a SkippedRow sets Reason without Code; build it with .Because(why)", fset.Position(lit.Pos()))
				}
				if keys["Code"] {
					coded++
				}
			case "EnsureResult":
				if keys["SkipReason"] && !keys["SkipWhy"] {
					t.Errorf("%s: an EnsureResult sets SkipReason without SkipWhy; use skippedFor", fset.Position(lit.Pos()))
				}
			}
			return true
		})
	}
	if literals < 8 {
		t.Fatalf("census saw %d SkippedRow literals, want at least 8: the walk is short", literals)
	}
	if coded < 1 {
		t.Fatal("census saw no SkippedRow literal that sets its code")
	}
}

func literalKeys(lit *ast.CompositeLit) map[string]bool {
	keys := map[string]bool{}
	for _, el := range lit.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok {
				keys[id.Name] = true
			}
		}
	}
	return keys
}

func TestProblemsCSVHeaderFollowsLang(t *testing.T) {
	rep := crmcontracts.ImportRunReport{Issues: []crmcontracts.ImportRowIssue{{
		Line: new(2), Reason: "english sentence",
		ReasonCode: new(crmcontracts.ImportReasonCode(migration.ReasonRowKeyEmpty)),
		Params:     &map[string]string{"column": "id"},
	}, {Line: new(3), Reason: "stored before codes"}}}
	for _, lang := range textlang.Shipped {
		if _, ok := importProblemsHeaders[lang]; !ok {
			t.Fatalf("no problems header for the shipped language %q", lang)
		}
	}
	var buf bytes.Buffer
	if err := writeImportProblems(&buf, rep, textlang.German); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(buf.String(), "\n")
	if lines[0] != "Datei,Zeile,Spalte,Problem,Wert" {
		t.Errorf("german header = %q", lines[0])
	}
	if !strings.Contains(lines[1], "ist leer, daher kann diese Zeile") {
		t.Errorf("coded row = %q, want its german sentence", lines[1])
	}
	if !strings.Contains(lines[2], "stored before codes") {
		t.Errorf("old row = %q, want the sentence it was stored with", lines[2])
	}
}

func TestImportRunAnswersProgressComputedOnTheDatabaseClock(t *testing.T) {
	read := time.Date(2026, 3, 2, 9, 10, 0, 0, time.UTC)
	ago := func(seconds int) *time.Time {
		at := read.Add(-time.Duration(seconds) * time.Second)
		return &at
	}
	run := migration.Run{
		Status: migration.StatusRunning, Phase: migration.PhaseImporting, PhaseObject: "contact",
		StartedAt: ago(90), ProgressAt: ago(2), HeartbeatAt: ago(1), ReadAt: read,
		Progress: &migration.Progress{
			Objects: []migration.ObjectProgress{{Object: "contact", RowsDone: 1100, RowsTotal: 6000}},
			Window: []migration.Sample{
				{At: read.Add(-62 * time.Second), Done: 100}, {At: read.Add(-2 * time.Second), Done: 1100},
			},
		},
	}
	var out crmcontracts.ImportRun
	applyProgress(&out, run)
	if out.ElapsedSeconds == nil || *out.ElapsedSeconds != 90 {
		t.Errorf("elapsed = %v, want 90 s against the read's clock", out.ElapsedSeconds)
	}
	if out.IdleSeconds == nil || *out.IdleSeconds != 2 || out.HeartbeatAgeSeconds == nil || *out.HeartbeatAgeSeconds != 1 {
		t.Errorf("idle %v, heartbeat %v, want 2 s and 1 s", out.IdleSeconds, out.HeartbeatAgeSeconds)
	}
	if out.RateRowsPerSecond == nil || out.EtaSecondsLow == nil || out.EtaSecondsHigh == nil || *out.EtaSecondsLow > *out.EtaSecondsHigh {
		t.Fatalf("rate %v, eta %v..%v, want a rate and a range", out.RateRowsPerSecond, out.EtaSecondsLow, out.EtaSecondsHigh)
	}
	if out.FinishedAt != nil {
		t.Errorf("finished_at = %v on a running run, want none", out.FinishedAt)
	}

	run.Status, run.UpdatedAt = migration.StatusComplete, *ago(30)
	out = crmcontracts.ImportRun{}
	applyProgress(&out, run)
	if out.FinishedAt == nil || !out.FinishedAt.Equal(run.UpdatedAt) {
		t.Errorf("finished_at = %v, want the last change %v of a run that ended without the column", out.FinishedAt, run.UpdatedAt)
	}
	if out.ElapsedSeconds == nil || *out.ElapsedSeconds != 60 || out.RateRowsPerSecond != nil || out.IdleSeconds != nil {
		t.Errorf("a finished run: elapsed %v rate %v idle %v, want 60 s and no live figures",
			out.ElapsedSeconds, out.RateRowsPerSecond, out.IdleSeconds)
	}
}

// A run that stopped moving reads the same elapsed time however long after
// the stop it is read, whichever final or at-rest status it is in.
func TestElapsedStopsWhereTheRunStoppedMoving(t *testing.T) {
	read := time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC)
	started := read.Add(-2*time.Hour - 10*time.Minute)
	stopped := read.Add(-2 * time.Hour)
	for _, status := range []string{migration.StatusFailed, migration.StatusAwaitingApproval} {
		run := migration.Run{Status: status, StartedAt: &started, UpdatedAt: stopped, ReadAt: read}
		var out crmcontracts.ImportRun
		applyProgress(&out, run)
		if out.ElapsedSeconds == nil || *out.ElapsedSeconds != 600 {
			t.Errorf("%s run read two hours later: elapsed = %v, want 600 s", status, out.ElapsedSeconds)
		}
		if out.FinishedAt != nil {
			t.Errorf("%s run: finished_at = %v, want none", status, out.FinishedAt)
		}
	}
}
