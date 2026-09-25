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
	Tier          string `json:"tier"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ThinkingLevel string `json:"thinking_level,omitempty"`
}

// aiCertPresetTask is one task as this preset serves it: the first ladder rung
// the preset binds, the model on that rung, and what the committed record for
// that pair says. Band and State are empty when nothing measured it.
type aiCertPresetTask struct {
	Task  string           `json:"task"`
	Label string           `json:"label"`
	Tier  string           `json:"tier"`
	Model aiCertBindingRef `json:"model"`
	Band  string           `json:"band"`
	State string           `json:"state"`
	// Runs and Passed are the record's pooled counts over every case of the
	// task, and the two case counts say which half of the rule held a grade
	// down: a case failing too many of its own runs, or its answers' quality.
	Runs                 int `json:"runs"`
	Passed               int `json:"passed"`
	CasesFailingOften    int `json:"cases_failing_often"`
	CasesBelowQualityBar int `json:"cases_below_quality_bar"`
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
			ThinkingLevel: binding.ThinkingLevel,
		})
	}
	return tiers
}

// attributeAICertPresets fills in each preset's per-task verdict from the
// records the document's site tables carry, so the by-preset section can never
// claim a band for a pair those tables do not show.
func attributeAICertPresets(presets []aiCertPreset, doc aiCertDoc, records []aicert.Record) []aiCertPreset {
	measured := aiCertTaskVerdicts(doc, records)
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
	row := aiCertPresetTask{Task: task, Label: ai.DisplayName(ai.Task(task))}
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
		row.Model = aiCertBindingRef{
			Provider: rung.Provider, Model: rung.Model, Env: preset.Profile, ThinkingLevel: rung.ThinkingLevel,
		}
		if seen, ok := measured[task+"\x00"+row.Model.label()]; ok {
			row.Band, row.State, row.Runs, row.Passed = seen.Band, seen.State, seen.Runs, seen.Passed
			row.CasesFailingOften, row.CasesBelowQualityBar = seen.CasesFailingOften, seen.CasesBelowQualityBar
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

// aiCertTaskVerdicts is one verdict per (task, binding): the record's OWN task
// verdict, which is what the runner computed over every case of the task, so
// the preset view and the task verdict are one answer. A pair enters only when
// a shipped site's table carries it, and its state is the worst any of those
// sites reports — one site measured on an older version is a task to re-check.
func aiCertTaskVerdicts(doc aiCertDoc, records []aicert.Record) map[string]aiCertPresetTask {
	byKey := map[string]aicert.Record{}
	for _, rec := range records {
		byKey[rec.Task+"\x00"+bindingRefOf(rec).label()] = rec
	}
	verdicts := map[string]aiCertPresetTask{}
	for _, site := range doc.Sites {
		for _, siteRec := range site.Records {
			key := site.Task + "\x00" + siteRec.Binding.label()
			if seen, found := verdicts[key]; found {
				if aiCertStateRank(siteRec.State) < aiCertStateRank(seen.State) {
					seen.State = siteRec.State
					verdicts[key] = seen
				}
				continue
			}
			rec := byKey[key]
			failing, belowQuality := aiCertCasesHoldingDown(rec)
			verdicts[key] = aiCertPresetTask{
				Band: rec.Verdict, State: siteRec.State, Runs: rec.Runs, Passed: rec.Passed,
				CasesFailingOften: failing, CasesBelowQualityBar: belowQuality,
			}
		}
	}
	return verdicts
}

// aiCertCasesHoldingDown counts the cases that miss each per-case half of the
// verdict rule. A row written before judge bands were recorded reveals its
// judge's band only when every run passed, since its stored verdict then has
// nothing else to reflect; otherwise its quality is unknown and not counted.
func aiCertCasesHoldingDown(rec aicert.Record) (failingOften, belowQuality int) {
	for _, sc := range rec.Scenarios {
		if aicert.CaseFallsShort(sc.Passed, sc.Runs) {
			failingOften++
		}
		band := sc.JudgeBand
		if band == "" && sc.Passed == sc.Runs {
			band = sc.Verdict
		}
		if band != "" && band != aicert.VerdictCertified {
			belowQuality++
		}
	}
	return failingOften, belowQuality
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
		for _, row := range p.Tasks {
			if row.Label == "" {
				t.Errorf("task %s has no display name in api/ai-tasks.yaml, so the page would name it by its id", row.Task)
			}
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

// assertAICertPresetsReadTheRecords holds every measured preset row to the
// committed record it names, looked up by the record's own key: the grade a
// preset shows is the task verdict the runner wrote, never a fold of sites.
func assertAICertPresetsReadTheRecords(t *testing.T, presets []aiCertPreset, records []aicert.Record) {
	t.Helper()
	byKey := map[string]aicert.Record{}
	for _, rec := range records {
		byKey[aicert.RecordKey(rec)] = rec
	}
	for _, p := range presets {
		for _, row := range p.Tasks {
			if row.Band == "" {
				continue
			}
			key := row.Task + "/" + row.Model.Provider + "/" + row.Model.Model + "/" + row.Model.Env
			rec, found := byKey[key]
			switch {
			case !found:
				t.Errorf("preset %s grades %s from no committed record %s", p.File, row.Task, key)
			case rec.Verdict != row.Band || rec.Runs != row.Runs || rec.Passed != row.Passed:
				t.Errorf("preset %s shows %s as %s, %d of %d; its record says %s, %d of %d",
					p.File, row.Task, row.Band, row.Passed, row.Runs, rec.Verdict, rec.Passed, rec.Runs)
			}
		}
	}
}

// A thinking level changes how a model answers, so a record run at one level
// grades only the preset whose rung sets the same level — in both directions.
func TestAPresetIsCreditedOnlyByARecordAtItsOwnThinkingLevel(t *testing.T) {
	const flashLite = "gemini-3.1-flash-lite-preview"
	cases := []struct {
		name, recordLevel, presetLevel, wantBand string
	}{
		{"a record at low does not grade a preset at the default", "low", "", ""},
		{"a record at the default does not grade a preset at low", "", "low", ""},
		{"a record at low grades a preset at low", "low", "low", aicert.VerdictCertified},
		{"a record at the default grades a preset at the default", "", "", aicert.VerdictCertified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := aicert.Record{
				Task: string(ai.TaskSummarize), Provider: "gemini", ServedModel: flashLite,
				EnvClass: string(ai.ProfileEUHosted), ThinkingLevel: tc.recordLevel,
				Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3,
			}
			doc := aiCertDoc{Sites: []aiCertSite{{
				Task:    rec.Task,
				Records: []aiCertRecord{{Binding: bindingRefOf(rec), State: aicert.StatusCurrent}},
			}}}
			preset := aiCertPreset{File: "flash-lite.yaml", Profile: rec.EnvClass, Tiers: []aiCertPresetTier{{
				Tier: string(ai.TierCheapCloud), Provider: "gemini", Model: flashLite, ThinkingLevel: tc.presetLevel,
			}}}
			got := attributeAICertPresets([]aiCertPreset{preset}, doc, []aicert.Record{rec})[0].Tasks[0]
			if got.Band != tc.wantBand {
				t.Errorf("preset at %q reads band %q from a record at %q, want %q",
					tc.presetLevel, got.Band, tc.recordLevel, tc.wantBand)
			}
		})
	}
}
