// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// The by-preset half of the certification page: what an operator actually gets
// if they copy one of the files under config/presets/.
//
// The rest of the page is organised by site and by binding, which answers "how
// did this model do". Nobody deploying this product answers that question: they
// choose a preset, and the preset chooses a model per tier on their behalf. So
// the band a task reaches for them is not the best band anything reached — it
// is the band of whatever model THEIR preset's ladder lands that task on, which
// can be worse, and can be nothing at all.
//
// Derived, never listed: the presets come from the directory, the ladder from
// ai.TaskLadder, and the bands from the same records the rest of the page
// reads. A preset added to the tree appears here without anyone remembering it,
// and a ladder rewritten in tasks_gen.go re-attributes every row.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// aiCertPresetDir is config/presets/ as this package reaches it, and
// aiCertPresetLink is the same directory as a reader of docs/reference/ follows.
var aiCertPresetDir = filepath.Join("..", "..", "..", "..", "config", "presets")

const aiCertPresetLink = corpusLinkPrefix + "config/presets/"

// aiCertPreset is one preset file folded over every task this build ships.
type aiCertPreset struct {
	File    string `json:"file"`
	Profile string `json:"profile"`
	// Tiers is the ladder rungs this preset binds, in the schema's own tier
	// order, so two presets read side by side line up.
	Tiers []aiCertPresetTier `json:"tiers"`
	Tasks []aiCertPresetTask `json:"tasks"`
	// Bands counts the tasks this preset can serve at each grade, and Untested
	// counts the ones it binds a model to that nothing has ever measured. The
	// two are different answers: a gap is not a failure, and a reader choosing
	// between presets has to be able to tell them apart.
	Bands    aiCertBands `json:"bands"`
	Untested int         `json:"untested"`
	Unbound  int         `json:"unbound"`
	// Unrecognised counts rows whose band this rollup has no column for. Always
	// zero today; a nonzero one means the verdict vocabulary grew and this
	// section is reporting less than it reads.
	Unrecognised int `json:"unrecognised"`
}

type aiCertPresetTier struct {
	Tier     string `json:"tier"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// aiCertPresetTask is one task as this preset serves it: the first ladder rung
// the preset binds, the model on that rung, and what the committed record for
// that pair says. Band and State are empty when nothing measured it.
type aiCertPresetTask struct {
	Task  string           `json:"task"`
	Tier  string           `json:"tier"`
	Model aiCertBindingRef `json:"model"`
	Band  string           `json:"band"`
	State string           `json:"state"`
}

// loadAICertPresets reads every preset in the directory through the same
// parser the product would, so a file this page describes is one an operator
// could boot.
func loadAICertPresets(t *testing.T) []aiCertPreset {
	t.Helper()
	entries, err := os.ReadDir(aiCertPresetDir)
	if err != nil {
		t.Fatalf("reading %s: %v", aiCertPresetDir, err)
	}
	var presets []aiCertPreset
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(aiCertPresetDir, entry.Name()))
		if err != nil {
			t.Fatalf("reading preset %s: %v", entry.Name(), err)
		}
		cfg, err := ai.ParsePreset(raw)
		if err != nil {
			t.Fatalf("preset %s does not parse, so this page would describe a binding nobody can deploy: %v",
				entry.Name(), err)
		}
		presets = append(presets, aiCertPreset{
			File: entry.Name(), Profile: string(cfg.Profile), Tiers: tiersOfPreset(cfg),
		})
	}
	// NOT a tolerated zero: an empty read renders a section claiming the tree
	// ships no presets, which is a statement rather than a missing one.
	if len(presets) == 0 {
		t.Fatalf("no presets found in %s — the by-preset section would report an empty tree as a finding", aiCertPresetDir)
	}
	return presets
}

// aiCertTierOrder is the ladder from cheapest rung to dearest. The map a config
// parses into has no order, and rendering a preset's rungs in map order would
// make two regenerations of an unchanged tree differ.
var aiCertTierOrder = []ai.Tier{
	ai.TierLocalSmall, ai.TierLocalLarge, ai.TierCheapCloud, ai.TierPremium, ai.TierFrontier,
}

func tiersOfPreset(cfg ai.RoutingConfig) []aiCertPresetTier {
	tiers := []aiCertPresetTier{}
	for _, tier := range aiCertTierOrder {
		binding, bound := cfg.Tiers[tier]
		if !bound {
			continue
		}
		tiers = append(tiers, aiCertPresetTier{
			Tier: string(tier), Provider: string(binding.Provider), Model: binding.Model,
		})
	}
	return tiers
}

// attributeAICertPresets fills in each preset's per-task verdict from the
// document's own records, so the by-preset section can never claim a band the
// per-site tables below it do not carry.
func attributeAICertPresets(presets []aiCertPreset, doc aiCertDoc) []aiCertPreset {
	measured := aiCertBandsByTaskBinding(doc)
	filled := make([]aiCertPreset, 0, len(presets))
	for _, preset := range presets {
		bound := map[string]aiCertPresetTier{}
		for _, tier := range preset.Tiers {
			bound[tier.Tier] = tier
		}
		preset.Tasks = []aiCertPresetTask{}
		for _, task := range aiCertTasksOf(doc) {
			preset.Tasks = append(preset.Tasks, presetTaskRow(task, preset, bound, measured))
		}
		for _, row := range preset.Tasks {
			countPresetTask(&preset, row)
		}
		filled = append(filled, preset)
	}
	return filled
}

func presetTaskRow(task string, preset aiCertPreset,
	bound map[string]aiCertPresetTier, measured map[string]aiCertPresetTask,
) aiCertPresetTask {
	row := aiCertPresetTask{Task: task}
	// The FIRST rung the preset binds, not the first rung the ladder names: a
	// task whose primary tier this preset leaves unbound is served by the next
	// one down, and reporting the primary would credit the preset with a model
	// it never reaches.
	for _, tier := range ai.TaskLadder(ai.Task(task)) {
		rung, ok := bound[string(tier)]
		if !ok {
			continue
		}
		row.Tier = rung.Tier
		row.Model = aiCertBindingRef{Provider: rung.Provider, Model: rung.Model, Env: preset.Profile}
		if seen, ok := measured[task+"\x00"+row.Model.label()]; ok {
			row.Band, row.State = seen.Band, seen.State
		}
		return row
	}
	return row
}

func countPresetTask(preset *aiCertPreset, row aiCertPresetTask) {
	switch {
	case row.Tier == "":
		preset.Unbound++
	case row.Band == "":
		preset.Untested++
	case row.Band == aicert.VerdictCertified:
		preset.Bands.Certified++
	case row.Band == aicert.VerdictSupportedDegraded:
		preset.Bands.SupportedDegraded++
	case row.Band == aicert.VerdictNotSupported:
		preset.Bands.NotSupported++
	default:
		// A verdict this rollup does not know is counted nowhere rather than
		// folded into the worst band: a new one added upstream should show as
		// a total that does not add up, which assertAICertPresetsAreAttributed
		// reports, not as a silent not_supported.
		preset.Unrecognised++
	}
}

// aiCertBandsByTaskBinding folds the document's per-site records down to one
// verdict per (task, binding). A record is written per task and repeated on
// every site of it, so the fold keeps the WORST band any site of the task
// reached: a preset's reader is told what the task does end to end, and one
// site that fails is a task that fails.
func aiCertBandsByTaskBinding(doc aiCertDoc) map[string]aiCertPresetTask {
	worst := map[string]aiCertPresetTask{}
	for _, site := range doc.Sites {
		for _, rec := range site.Records {
			key := site.Task + "\x00" + rec.Binding.label()
			seen, found := worst[key]
			if found && bandRank(seen.Band) <= bandRank(rec.Band) {
				continue
			}
			worst[key] = aiCertPresetTask{Band: rec.Band, State: rec.State}
		}
	}
	return worst
}

func aiCertTasksOf(doc aiCertDoc) []string {
	seen := map[string]bool{}
	var tasks []string
	for _, site := range doc.Sites {
		if !seen[site.Task] {
			seen[site.Task] = true
			tasks = append(tasks, site.Task)
		}
	}
	sort.Strings(tasks)
	return tasks
}

// writeAICertPresets renders the section, and renders it FIRST: a reader
// arriving at this page is deciding what to deploy, and the per-site tables
// below answer a question they have to already know the answer to.
func writeAICertPresets(page *strings.Builder, presets []aiCertPreset) {
	page.WriteString("## What each preset gives you\n\n")
	page.WriteString("An operator deploys a [preset](" + aiCertPresetLink + "README.md), not a model. The preset binds a\n")
	page.WriteString("model per tier, and each task walks its own ladder until it reaches a tier the\n")
	page.WriteString("preset bound — so the grade a task gets under a preset is that model's grade,\n")
	page.WriteString("never the best grade anything reached. `untested` is a gap, not a failure: the\n")
	page.WriteString("preset binds a model there and no paid run has measured it yet.\n\n")
	page.WriteString("| Preset | Profile | `certified` | `supported_degraded` | `not_supported` | `untested` | Unbound |\n")
	page.WriteString("|---|---|---:|---:|---:|---:|---:|\n")
	for _, p := range presets {
		fmt.Fprintf(page, "| [`%s`](%s%s) | `%s` | %d | %d | %d | %d | %d |\n",
			p.File, aiCertPresetLink, p.File, p.Profile,
			p.Bands.Certified, p.Bands.SupportedDegraded, p.Bands.NotSupported, p.Untested, p.Unbound)
	}
	page.WriteString("\nUnbound counts tasks whose whole ladder this preset leaves empty — the router\n")
	page.WriteString("has nothing to call, so the feature is off rather than degraded.\n\n")
	for _, p := range presets {
		writeAICertPresetDetail(page, p)
	}
}

func writeAICertPresetDetail(page *strings.Builder, p aiCertPreset) {
	fmt.Fprintf(page, "### `%s`\n\n", p.File)
	page.WriteString("| Tier | Provider | Model |\n|---|---|---|\n")
	for _, tier := range p.Tiers {
		fmt.Fprintf(page, "| `%s` | `%s` | `%s` |\n", tier.Tier, tier.Provider, tier.Model)
	}
	page.WriteString("\n| Task | Served on | Model | Band | State |\n|---|---|---|---|---|\n")
	for _, row := range p.Tasks {
		fmt.Fprintf(page, "| `%s` | %s | %s | %s | %s |\n",
			row.Task, aiCertCell(row.Tier), aiCertCell(row.Model.Model),
			aiCertBandCell(row), aiCertCell(row.State))
	}
	page.WriteString("\n")
}

// aiCertCell renders an unmeasured or unbound value as the page's own dash
// rather than as an empty table cell, which reads as a rendering fault.
func aiCertCell(value string) string {
	if value == "" {
		return "-"
	}
	return "`" + value + "`"
}

func aiCertBandCell(row aiCertPresetTask) string {
	if row.Tier == "" {
		return "`unbound`"
	}
	if row.Band == "" {
		return "`untested`"
	}
	return "`" + row.Band + "`"
}

// assertAICertPresetsAreAttributed is the guard the drift check cannot be: a
// builder that quietly stopped resolving ladders would emit a section of
// dashes, and the committed copy rendered by the same builder would match it.
//
// So every preset is asked the two questions a silent failure answers wrongly:
// does it cover every task the page ships, and does at least one preset reach a
// measured band — because "nothing is certified anywhere" is a claim, and this
// build's records say otherwise.
func assertAICertPresetsAreAttributed(t *testing.T, presets []aiCertPreset, doc aiCertDoc) {
	t.Helper()
	tasks := aiCertTasksOf(doc)
	for _, p := range presets {
		if len(p.Tasks) != len(tasks) {
			t.Errorf("preset %s reports %d tasks and the page ships %d", p.File, len(p.Tasks), len(tasks))
		}
		if len(p.Tiers) == 0 {
			t.Errorf("preset %s binds no tier, so every task under it would read as unbound", p.File)
		}
		if p.Unrecognised > 0 {
			t.Errorf("preset %s carries %d row(s) whose band this rollup has no column for", p.File, p.Unrecognised)
		}
		// PER PRESET, not summed across them. A total hides the failure this
		// asks about: attribution keys a preset's profile against a record's
		// env, so one typo'd `profile:` turns that preset entirely `untested`
		// while every other preset keeps the sum comfortably positive.
		if measured := p.Bands.Certified + p.Bands.SupportedDegraded + p.Bands.NotSupported; measured == 0 {
			t.Errorf("preset %s reaches no measured band at all — every task reads untested or unbound, "+
				"which is a claim about the product if true and a broken join if not", p.File)
		}
	}
}
