// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// docs/reference/open-vs-closed-models.md sets the open-weight model setups
// beside the closed ones, with Gemini as the standard.
// It shows how many features each carries and what one run of each costs.
// It is rendered from the certification payload, the records and the price
// sheet, so no figure on it is typed by hand.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

var updateOpenVsClosed = flag.Bool("update-open-vs-closed", false,
	"rewrite docs/reference/open-vs-closed-models.md from the certification payload, the records and the price sheet")

const (
	openVsClosedFile    = "open-vs-closed-models.md"
	openVsClosedCommand = "go test ./internal/compose/aicert/ -run TestTheOpenVsClosedPageIsPublished -update-open-vs-closed"
	openVsClosedBase    = "gemini_cloud.yaml"
)

// openVsClosedDescribes is the code that decides what the page states.
const openVsClosedDescribes = "backend/internal/compose/aicert/openvsclosedpage_test.go " +
	"backend/internal/compose/aicert/records backend/internal/modules/ai/pricing.go config/presets"

// openVsClosedSetup is one setup the page compares, keyed by its preset file.
// A renamed preset then fails loudly instead of dropping off the page.
type openVsClosedSetup struct {
	file, weights string
	open          bool
}

var openVsClosedSetups = []openVsClosedSetup{
	{openVsClosedBase, "closed", false},
	{"anthropic_cloud.yaml", "closed", false},
	{"openrouter_openai.yaml", "closed", false},
	{"consumer_class_brokered.yaml", "open (Gemma 4)", true},
	{"openrouter_cloud.yaml", "open (gpt-oss) with Mistral on the upper tiers and closed Claude on signal extraction", true},
}

// openVsClosedPricedOn is only the date SeedModelRates stamps on its rows.
// It selects no rate: a cost moves when a row's amount in pricing.go moves.
var openVsClosedPricedOn = time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)

// openVsClosedGrade names a band the way the certification page does, and
// reports a band it does not know as absent.
func openVsClosedGrade(band string) (string, bool) {
	switch band {
	case "certified":
		return "Ready", true
	case "supported_degraded":
		return "Usable with care", true
	case "not_supported":
		return "Not reliable yet", true
	}
	return "", false
}

// costMicro is what one run of a feature's test cases costs at list price, in
// micro-USD. It prices the mean tokens the record measured through the
// product's own PriceCall. A model's long-prompt band then applies as it does
// in the cost report. The cache buckets are left out: a first call pays none.
func costMicro(rec aicert.Record, rates map[string]ai.ModelRate) (int64, bool) {
	rate, ok := rates[rec.Provider+"/"+rec.ServedModel]
	if !ok {
		return 0, false
	}
	return ai.PriceCall(ai.Usage{TokensIn: rec.MeanTokensIn, TokensOut: rec.MeanTokensOut}, rate), true
}

func usd(micro int64) string { return fmt.Sprintf("$%.4f", float64(micro)/1e6) }

// openVsClosedRow is one feature on one setup.
type openVsClosedRow struct {
	label, model, grade string
	costMicro           int64
	costed, stale       bool
}

func TestTheOpenVsClosedPageIsPublished(t *testing.T) {
	cert := readWhichAIPayload[whichAICert](t, "ai-certification.json")
	records, err := aicert.LoadRecords("records")
	if err != nil {
		t.Fatal(err)
	}
	rates := map[string]ai.ModelRate{}
	for _, r := range ai.SeedModelRates(openVsClosedPricedOn) {
		rates[r.Provider+"/"+r.ModelID] = r
	}
	byKey := map[string]aicert.Record{}
	for _, r := range records {
		byKey[r.Task+"|"+r.Provider+"|"+r.ServedModel+"|"+r.EnvClass] = r
	}

	setups := map[string]map[string]openVsClosedRow{}
	titles := map[string]whichAISetup{}
	for _, want := range openVsClosedSetups {
		found := false
		for _, s := range cert.Presets {
			if s.File != want.file {
				continue
			}
			found = true
			titles[want.file] = s
			rows := map[string]openVsClosedRow{}
			for _, task := range s.Tasks {
				row := openVsClosedRow{label: task.Label, model: task.Model.Model, grade: "Not measured"}
				if g, ok := openVsClosedGrade(task.Band); ok {
					row.grade = g
					row.stale = task.State != "current"
				}
				if rec, ok := byKey[task.Task+"|"+task.Model.Provider+"|"+task.Model.Model+"|"+task.Model.Env]; ok {
					row.costMicro, row.costed = costMicro(rec, rates)
				}
				rows[task.Task] = row
			}
			setups[want.file] = rows
		}
		if !found {
			t.Fatalf("the page compares %s, and the certification payload has no such setup", want.file)
		}
	}

	page := renderOpenVsClosed(setups, titles)
	path := filepath.Join(whichAIReference, openVsClosedFile)
	if *updateOpenVsClosed {
		if err := os.WriteFile(path, page, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if !bytes.Equal(got, page) {
		t.Errorf("%s is out of date with the records; run `cd backend && %s`", path, openVsClosedCommand)
	}
}

// costedTotal sums the cost of one run of every feature the setup prices.
// It also counts the features it could not price, so a total never reads as
// complete when it is not.
func costedTotal(rows map[string]openVsClosedRow) (total int64, costed, all int) {
	for _, r := range rows {
		all++
		if r.costed {
			costed++
			total += r.costMicro
		}
	}
	return total, costed, all
}

// againstBase is this setup's cost over the features both it and the
// standard price, as a multiple of the standard's cost over the same features.
// Two totals over different features would compare unlike work.
func againstBase(rows, base map[string]openVsClosedRow) string {
	var own, std int64
	for task, r := range rows {
		if b := base[task]; r.costed && b.costed {
			own += r.costMicro
			std += b.costMicro
		}
	}
	if std == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f×", float64(own)/float64(std))
}

func renderOpenVsClosed(setups map[string]map[string]openVsClosedRow, titles map[string]whichAISetup) []byte {
	var p strings.Builder
	p.WriteString("<!-- describes: " + openVsClosedDescribes + " -->\n# Open and closed models compared\n\n")
	p.WriteString("<!-- Generated by `cd backend && " + openVsClosedCommand + "` from the certification payload, " +
		"the committed records and the price sheet; do not edit by hand. -->\n\n")
	p.WriteString("This page sets the open-weight model setups beside the closed ones, with Gemini as the standard. " +
		"It answers two questions: how many Margince AI features each setup carries, and what one run of each costs. " +
		"Where a model is hosted is a later question, and nothing here measures it. " +
		"The grades come from [ai-certification.md](ai-certification.md), which explains how each is earned.\n\n")

	base := setups[openVsClosedBase]

	p.WriteString("## The setups side by side\n\n")
	p.WriteString("| Setup | Weights | Ready | Usable with care | Not reliable yet | Not measured | Cost of one run of each feature | Against Gemini |\n")
	p.WriteString("|---|---|---:|---:|---:|---:|---:|---:|\n")
	for _, s := range openVsClosedSetups {
		t := titles[s.file]
		total, costed, all := costedTotal(setups[s.file])
		cost := usd(total)
		if costed < all {
			cost += fmt.Sprintf(" (%d of %d features priced)", costed, all)
		}
		fmt.Fprintf(&p, "| %s | %s | %d | %d | %d | %d | %s | %s |\n", t.Title, s.weights,
			t.Bands.Ready, t.Bands.UsableWithCare, t.Bands.NotReliableYet, t.NotMeasured, cost, againstBase(setups[s.file], base))
	}
	p.WriteString("\nCost basis: one run is the mean tokens that feature's test cases used, priced at the list price in " +
		"[`pricing.go`](../../backend/internal/modules/ai/pricing.go): input and output only, no cache discount. " +
		"It compares models on equal work. It is not a forecast of a bill, because a real workspace calls the " +
		"features in a different mix. The last column compares only the features both setups price, " +
		"so its Gemini total differs from row to row.\n\n")

	for _, s := range openVsClosedSetups {
		if s.open {
			writeOpenVsClosedFeatures(&p, titles[s.file], setups[s.file], base, titles[openVsClosedBase])
		}
	}

	p.WriteString("## How to read the comparison\n\n")
	p.WriteString("- The judge can differ from one result to the next. A second model scores answer quality, and " +
		"[ai-certification.md](ai-certification.md) names the judge of each result. Trust \"tries right\" there " +
		"most, because a mechanical check decides it.\n")
	p.WriteString("- A feature with no price is left out of the total, and the table says how many.\n")
	p.WriteString("- \"Not measured\" means no test ran. It is not a failed test. " +
		"Gemma 4 document extraction reads so because the binding declares no attachment input, " +
		"and the runner refuses the test before it calls a model.\n")
	p.WriteString("- A grade belongs to the setup that was tested: this model, served through the broker with its " +
		"own routing. The same model on a host you run yourself needs its own measurement.\n")
	return []byte(p.String())
}

func writeOpenVsClosedFeatures(p *strings.Builder, open whichAISetup, rows, base map[string]openVsClosedRow, std whichAISetup) {
	fmt.Fprintf(p, "## Feature by feature: %s\n\nThe standard is %s.\n\n", open.Title, std.Title)
	p.WriteString("| Feature | Gemini | Open setup | Cost of one run: Gemini | Cost of one run: open |\n|---|---|---|---:|---:|\n")
	var behind, behindOld []string
	for _, t := range std.Tasks {
		b, o := rowOrNotMeasured(base, t.Task), rowOrNotMeasured(rows, t.Task)
		fmt.Fprintf(p, "| %s | %s (`%s`) | %s (`%s`) | %s | %s |\n", t.Label,
			gradeCell(b), b.model, gradeCell(o), o.model, costCell(b), costCell(o))
		if b.grade == "Ready" && o.grade != "Ready" {
			if b.stale || o.stale {
				behindOld = append(behindOld, t.Label)
			} else {
				behind = append(behind, t.Label)
			}
		}
	}
	writeGapList(p, "\nGemini is Ready and this setup is not, on measurements that are both current:\n\n", behind)
	writeGapList(p, "Gemini is Ready and this setup is not, but one of the two measurements waits for a re-check:\n\n", behindOld)
}

func writeGapList(p *strings.Builder, lead string, labels []string) {
	if len(labels) == 0 {
		return
	}
	p.WriteString(lead)
	for _, label := range labels {
		fmt.Fprintf(p, "- %s\n", label)
	}
	p.WriteString("\n")
}

// rowOrNotMeasured reads a feature a setup does not bind as "Not measured".
// A zero row would print an empty cell instead.
func rowOrNotMeasured(rows map[string]openVsClosedRow, task string) openVsClosedRow {
	if r, ok := rows[task]; ok {
		return r
	}
	return openVsClosedRow{grade: "Not measured", model: "no model bound"}
}

// gradeCell keeps the re-check marker the certification page shows, so an old
// measurement does not read as a current one.
func gradeCell(r openVsClosedRow) string {
	if r.stale {
		return r.grade + " · re-check pending"
	}
	return r.grade
}

func costCell(r openVsClosedRow) string {
	if !r.costed {
		return "—"
	}
	return usd(r.costMicro)
}
