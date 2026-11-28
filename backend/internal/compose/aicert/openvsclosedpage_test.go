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
type openVsClosedSetup struct{ file, weights string }

var openVsClosedSetups = []openVsClosedSetup{
	{openVsClosedBase, "closed"},
	{"anthropic_cloud.yaml", "closed"},
	{"openrouter_openai.yaml", "closed"},
	{"consumer_class_brokered.yaml", "open (Gemma 4)"},
	{"openrouter_cloud.yaml", "open (gpt-oss) with Mistral on the upper tiers"},
}

var openVsClosedGrade = map[string]string{
	"certified": "Ready", "supported_degraded": "Usable with care", "not_supported": "Not reliable yet",
}

// costMicro is what one run of a feature's test cases costs at list price, in
// micro-USD, from the mean tokens its record measured. Cache discounts are
// left out: the cheapest bucket is not the one a first call pays.
func costMicro(rec aicert.Record, rates map[string]ai.ModelRate) (int64, bool) {
	rate, ok := rates[rec.Provider+"/"+rec.ServedModel]
	if !ok {
		return 0, false
	}
	return (int64(rec.MeanTokensIn)*rate.InputPerMTokMicroUSD + int64(rec.MeanTokensOut)*rate.OutputPerMTokMicroUSD) / 1_000_000, true
}

func usd(micro int64) string { return fmt.Sprintf("$%.4f", float64(micro)/1e6) }

// openVsClosedRow is one feature on one setup.
type openVsClosedRow struct {
	label, model, grade string
	costMicro           int64
	costed              bool
}

func TestTheOpenVsClosedPageIsPublished(t *testing.T) {
	cert := readWhichAIPayload[whichAICert](t, "ai-certification.json")
	records, err := aicert.LoadRecords(filepath.Join("records"))
	if err != nil {
		t.Fatal(err)
	}
	rates := map[string]ai.ModelRate{}
	for _, r := range ai.SeedModelRates(time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)) {
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
				if g, ok := openVsClosedGrade[task.Band]; ok {
					row.grade = g
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
	if err != nil || !bytes.Equal(got, page) {
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
	p.WriteString("\n**Cost basis.** One run is the mean tokens that feature's test cases used, priced at the list price in " +
		"[`pricing.go`](../../backend/internal/modules/ai/pricing.go): input and output only, no cache discount. " +
		"It compares models on equal work. It is not a forecast of a bill, because a real workspace calls the " +
		"features in a different mix.\n\n")

	for _, s := range openVsClosedSetups[3:] {
		writeOpenVsClosedFeatures(&p, titles[s.file], setups[s.file], base, titles[openVsClosedBase])
	}

	p.WriteString("## How to read the comparison\n\n")
	p.WriteString("- **The judges differ.** A second model scores answer quality, and each family was scored by its own " +
		"judge. Compare the grades with that in mind, and trust \"tries right\" in " +
		"[ai-certification.md](ai-certification.md) most, because a mechanical check decides it.\n")
	p.WriteString("- **A feature with no price is left out of the total**, and the table says how many. " +
		"A setup that cannot carry a feature at all reads \"Not measured\".\n")
	p.WriteString("- **Open weights are measured through a broker here.** The grade belongs to the model, and the " +
		"serving, speed and price of a host you run yourself are separate measurements.\n")
	return []byte(p.String())
}

func writeOpenVsClosedFeatures(p *strings.Builder, open whichAISetup, rows, base map[string]openVsClosedRow, std whichAISetup) {
	fmt.Fprintf(p, "## Feature by feature: %s\n\nThe standard is %s.\n\n", open.Title, std.Title)
	p.WriteString("| Feature | Gemini | Open setup | Cost of one run: Gemini | Cost of one run: open |\n|---|---|---|---:|---:|\n")
	var behind []string
	for _, t := range std.Tasks {
		b, o := base[t.Task], rows[t.Task]
		fmt.Fprintf(p, "| %s | %s (`%s`) | %s (`%s`) | %s | %s |\n", t.Label,
			b.grade, b.model, o.grade, o.model, costCell(b), costCell(o))
		if b.grade == "Ready" && o.grade != "Ready" {
			behind = append(behind, t.Label)
		}
	}
	fmt.Fprintf(p, "\n**Where Gemini is Ready and this setup is not (%d):** %s.\n\n", len(behind), strings.Join(behind, ", "))
}

func costCell(r openVsClosedRow) string {
	if !r.costed {
		return "—"
	}
	return usd(r.costMicro)
}
