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
	"github.com/margince/margince/backend/internal/shared/gatekit"
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
	// binding is the whole rung: which floor a site asks of it depends on its
	// host and routing too, which the page does not show.
	binding ai.ProviderConfig
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
	// Fallback is the rung a failed call on Tier falls to, or nil when the
	// ladder binds no further rung.
	Fallback *aiCertFallback `json:"fallback,omitempty"`
	// Abandoned counts the measured runs the upstream broke off mid-answer:
	// each is a call the router would have handed to Fallback.
	Abandoned int `json:"abandoned"`
	// Runs and Passed are the record's pooled counts over every case of the
	// task, and the two case counts say which half of the rule held a grade
	// down: a case failing too many of its own runs, or its answers' quality.
	Runs                 int `json:"runs"`
	Passed               int `json:"passed"`
	CasesFailingOften    int `json:"cases_failing_often"`
	CasesBelowQualityBar int `json:"cases_below_quality_bar"`
	// MeasuredOn is the binding whose record grades this rung when it is not
	// the rung's own (aicert.MeasuredAs), so the page can say so.
	MeasuredOn *aiCertBindingRef `json:"measured_on,omitempty"`
	// record is the measuring record, which grades a rung only where
	// aicert.RecordMeasures says it measured that rung.
	record aicert.Record
}

// aiCertFallback is the next bound rung of a task's ladder and what the
// committed record for its model on this task says.
type aiCertFallback struct {
	Tier  string           `json:"tier"`
	Model aiCertBindingRef `json:"model"`
	// SameModel says the rung binds the model that just failed, so the walk
	// asks it again rather than reaching another.
	SameModel bool   `json:"same_model"`
	Band      string `json:"band"`
	State     string `json:"state"`
	// MeasuredOn is as on aiCertPresetTask: the binding whose record grades
	// this rung when it is not the rung's own.
	MeasuredOn *aiCertBindingRef `json:"measured_on,omitempty"`
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
			ThinkingLevel: binding.ThinkingLevel, binding: binding,
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
		preset.Tasks = []aiCertPresetTask{}
		for _, task := range aiCertTasksOf(doc) {
			preset.Tasks = append(preset.Tasks, presetTaskRow(task, preset, measured))
		}
		for _, row := range preset.Tasks {
			countPresetTask(&preset, row)
		}
		filled = append(filled, preset)
	}
	return filled
}

// presetTaskRow walks the task's ladder as a routed certification run does:
// the first rung the preset binds answers, and the next is where a failed call
// falls. A task whose primary tier the preset leaves unbound is served by the
// next one down, so reporting the primary would credit a model it never reaches.
func presetTaskRow(task string, preset aiCertPreset, measured map[string]aiCertPresetTask) aiCertPresetTask {
	row := aiCertPresetTask{Task: task, Label: ai.DisplayName(ai.Task(task))}
	rungs := aicert.RungsBound(preset.routing(), ai.Task(task))
	if len(rungs) == 0 {
		return row
	}
	first := rungs[0]
	row.Tier, row.Model = string(first.Tier), presetBindingRef(first.Binding, preset.Profile)
	if seen, on, ok := measuredOn(task, first.Binding, row.Model, measured); ok {
		row.MeasuredOn = on
		row.Band, row.State, row.Runs, row.Passed = seen.Band, seen.State, seen.Runs, seen.Passed
		row.CasesFailingOften, row.CasesBelowQualityBar = seen.CasesFailingOften, seen.CasesBelowQualityBar
		row.Abandoned = seen.Abandoned
	}
	if len(rungs) > 1 {
		next := rungs[1]
		row.Fallback = &aiCertFallback{
			Tier: string(next.Tier), Model: presetBindingRef(next.Binding, preset.Profile),
			SameModel: next.Binding.Provider == first.Binding.Provider && next.Binding.Model == first.Binding.Model,
		}
		if seen, on, ok := measuredOn(task, next.Binding, row.Fallback.Model, measured); ok {
			row.Fallback.Band, row.Fallback.State, row.Fallback.MeasuredOn = seen.Band, seen.State, on
		}
	}
	return row
}

// measuredOn is the record that grades a rung, by the rule the runner's
// STALE_ONLY skip and the readiness report read too (aicert.RecordMeasures):
// the rung's own record, else the one aicert.MeasuredAs names, which it
// returns so the page can say where the grade was measured.
func measuredOn(task string, binding ai.ProviderConfig, ref aiCertBindingRef,
	measured map[string]aiCertPresetTask,
) (aiCertPresetTask, *aiCertBindingRef, bool) {
	if seen, ok := measured[aiCertRouteKey(task, ref)]; ok && aicert.RecordMeasures(seen.record, binding, ai.Profile(ref.Env), ai.Task(task)) {
		return seen, nil, true
	}
	as, env := aicert.MeasuredAs(binding, ai.Profile(ref.Env))
	if as.Provider == binding.Provider {
		return aiCertPresetTask{}, nil, false
	}
	on := presetBindingRef(as, string(env))
	seen, ok := measured[aiCertRouteKey(task, on)]
	if !ok || !aicert.RecordMeasures(seen.record, binding, ai.Profile(ref.Env), ai.Task(task)) {
		return aiCertPresetTask{}, nil, false
	}
	return seen, &on, true
}

func presetBindingRef(binding ai.ProviderConfig, profile string) aiCertBindingRef {
	return aiCertBindingRef{
		Provider: binding.Provider, Model: binding.Model, Env: profile, ThinkingLevel: binding.ThinkingLevel,
	}
}

// routing is the preset as the router reads it: its profile and its rungs.
func (p aiCertPreset) routing() ai.RoutingConfig {
	cfg := ai.RoutingConfig{Profile: ai.Profile(p.Profile), Tiers: map[ai.Tier]ai.ProviderConfig{}}
	for _, tier := range p.Tiers {
		cfg.Tiers[ai.Tier(tier.Tier)] = tier.binding
	}
	return cfg
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
		byKey[aiCertRouteKey(rec.Task, bindingRefOf(rec))] = rec
	}
	verdicts := map[string]aiCertPresetTask{}
	for _, site := range doc.Sites {
		for _, siteRec := range site.Records {
			key := aiCertRouteKey(site.Task, siteRec.Binding)
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
				CasesFailingOften: failing, CasesBelowQualityBar: belowQuality, Abandoned: aiCertAbandoned(rec),
				record: rec,
			}
		}
	}
	return verdicts
}

func aiCertAbandoned(rec aicert.Record) int {
	abandoned := 0
	for _, sc := range rec.Scenarios {
		abandoned += sc.Abandoned
	}
	return abandoned
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
		band := sc.CaseJudgeBand()
		if band == "" && sc.Passed == sc.Runs {
			band = sc.CaseVerdict()
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
	}
	for _, problem := range unmeasuredPresetProblems(t, presets, aiCertRecordedModels(doc), aiCertUnmeasuredWaivers) {
		t.Error(problem)
	}
	aiCertUnmeasuredWaivers.AssertAllMatched(t)
}

// aiCertUnmeasuredWaivers names the presets allowed to reach no measured band,
// each with why. An entry is stale once its preset is measured or gone
// (AssertAllMatched), or once any record measures one of its models.
var aiCertUnmeasuredWaivers = gatekit.Waive(map[string]string{})

// unmeasuredPresetProblems asks, PER PRESET and not summed across them,
// whether each reaches a measured band. A total hides the failure this asks
// about: attribution keys a preset's profile against a record's env, so one
// typo'd `profile:` turns that preset entirely `untested` while every other
// preset keeps the sum comfortably positive.
func unmeasuredPresetProblems(t testing.TB, presets []aiCertPreset, recorded map[string]bool, waivers *gatekit.Waivers[string]) []string {
	var problems []string
	for _, p := range presets {
		if p.Bands.Certified+p.Bands.SupportedDegraded+p.Bands.NotSupported > 0 {
			continue
		}
		switch {
		case !waivers.Waived(t, p.File):
			problems = append(problems, fmt.Sprintf("preset %s reaches no measured band at all — every task reads untested or unbound, "+
				"which is a claim about the product if true and a broken join if not", p.File))
		case bindsAMeasuredModel(p, recorded):
			problems = append(problems, fmt.Sprintf("preset %s is waived as unmeasured, but a record measures one of its models, "+
				"so its zero is a broken join — fix the attribution, then delete its entry from aiCertUnmeasuredWaivers", p.File))
		}
	}
	return problems
}

// aiCertRecordedModels answers whether some record measured a provider and
// model, under any profile.
func aiCertRecordedModels(doc aiCertDoc) map[string]bool {
	recorded := map[string]bool{}
	for _, site := range doc.Sites {
		for _, rec := range site.Records {
			recorded[rec.Binding.Provider+"\x00"+rec.Binding.Model] = true
		}
	}
	return recorded
}

func bindsAMeasuredModel(p aiCertPreset, recorded map[string]bool) bool {
	for _, tier := range p.Tiers {
		if recorded[tier.Provider+"\x00"+tier.Model] {
			return true
		}
	}
	return false
}

func TestAnUnmeasuredPresetFailsUnlessWaivedAndItsWaiverFailsOnceARecordExists(t *testing.T) {
	t.Parallel()
	unmeasured := aiCertPreset{File: "fresh.yaml", Tiers: []aiCertPresetTier{{Tier: "premium", Provider: "p", Model: "m"}}}
	for name, tc := range map[string]struct {
		recorded map[string]bool
		waivers  map[string]string
		want     string
	}{
		"unwaived": {map[string]bool{}, map[string]string{}, "reaches no measured band"},
		"stale":    {map[string]bool{"p\x00m": true}, map[string]string{"fresh.yaml": "no certification run has been paid for its models yet"}, "broken join"},
		"current":  {map[string]bool{"other\x00m": true}, map[string]string{"fresh.yaml": "no certification run has been paid for its models yet"}, ""},
	} {
		problems := unmeasuredPresetProblems(t, []aiCertPreset{unmeasured}, tc.recorded, gatekit.Waive(tc.waivers))
		switch {
		case tc.want == "" && len(problems) != 0:
			t.Errorf("%s: a current waiver still failed: %q", name, problems)
		case tc.want != "" && (len(problems) != 1 || !strings.Contains(problems[0], tc.want)):
			t.Errorf("%s: problems = %q, want exactly one saying %q", name, problems, tc.want)
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
			graded := row.Model
			if row.MeasuredOn != nil {
				graded = *row.MeasuredOn
			}
			key := row.Task + "/" + graded.Provider + "/" + graded.Model + "/" + graded.Env
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
				binding: ai.ProviderConfig{Provider: "gemini", Model: flashLite, ThinkingLevel: tc.presetLevel},
			}}}
			got := attributeAICertPresets([]aiCertPreset{preset}, doc, []aicert.Record{rec})[0].Tasks[0]
			if got.Band != tc.wantBand {
				t.Errorf("preset at %q reads band %q from a record at %q, want %q",
					tc.presetLevel, got.Band, tc.recordLevel, tc.wantBand)
			}
		})
	}
}

// A site the contract tells to think runs at that level on a rung that sends
// it, so a preset is graded by the record whose sites ran at the levels the
// preset would serve them at — and not by one run before the level existed.
func TestAPresetIsCreditedOnlyByARecordAtTheLevelsItServesEachSite(t *testing.T) {
	const flashLite = "gemini-3.1-flash-lite"
	served := ai.SiteThinkingLevels(ai.ProviderConfig{Provider: "gemini", Model: flashLite}, ai.TaskColdStart)
	if len(served) == 0 {
		t.Fatal("no cold_start site declares a level, so this test grades nothing")
	}
	cases := []struct {
		name         string
		siteThinking map[string]string
		wantBand     string
	}{
		{"a record at the sites' levels grades the preset", served, aicert.VerdictCertified},
		{"a record from before the sites declared one does not", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := aicert.Record{
				Task: string(ai.TaskColdStart), Provider: "gemini", ServedModel: flashLite,
				EnvClass: string(ai.ProfileCloudFrontier), SiteThinking: tc.siteThinking,
				Verdict: aicert.VerdictCertified, Runs: 3, Passed: 3,
			}
			doc := aiCertDoc{Sites: []aiCertSite{{
				Task:    rec.Task,
				Records: []aiCertRecord{{Binding: bindingRefOf(rec), State: aicert.StatusCurrent}},
			}}}
			preset := aiCertPreset{File: "gemini.yaml", Profile: rec.EnvClass, Tiers: []aiCertPresetTier{{
				Tier: string(ai.TierCheapCloud), Provider: "gemini", Model: flashLite,
				binding: ai.ProviderConfig{Provider: "gemini", Model: flashLite},
			}}}
			got := attributeAICertPresets([]aiCertPreset{preset}, doc, []aicert.Record{rec})[0].Tasks[0]
			if got.Band != tc.wantBand {
				t.Errorf("preset reads band %q from a record with site levels %v, want %q", got.Band, tc.siteThinking, tc.wantBand)
			}
		})
	}
}

// attributeOneTask is one preset, binding tiers under profile, over a document
// that ships task alone and carries records as its current measurements.
func attributeOneTask(task ai.Task, profile ai.Profile, tiers map[ai.Tier]ai.ProviderConfig,
	records ...aicert.Record,
) aiCertPreset {
	site := aiCertSite{Task: string(task)}
	for _, rec := range records {
		site.Records = append(site.Records, aiCertRecord{Binding: bindingRefOf(rec), State: aicert.StatusCurrent})
	}
	preset := aiCertPreset{File: "p.yaml", Profile: string(profile), Tiers: tiersOfPreset(ai.RoutingConfig{Tiers: tiers})}
	return attributeAICertPresets([]aiCertPreset{preset}, aiCertDoc{Sites: []aiCertSite{site}}, records)[0]
}

func measuredRecord(task ai.Task, binding ai.ProviderConfig, profile ai.Profile, verdict string, abandoned int) aicert.Record {
	return aicert.Record{
		Task: string(task), Provider: binding.Provider, ServedModel: binding.Model, EnvClass: string(profile),
		Verdict: verdict, Runs: 6, Passed: 6 - abandoned,
		Scenarios: []aicert.ScenarioRecord{{Runs: 6, Passed: 6 - abandoned, Abandoned: abandoned}},
	}
}

// A preset's row names the model that answers a feature and the model a failed
// call falls to, graded on the feature by that model's own record or said to be
// unmeasured; a rung binding the same model is no fallback, and a ladder with
// no further bound rung names none.
func TestAPresetRowNamesTheModelThatAnswersAndTheOneAFailedCallFallsTo(t *testing.T) {
	task := ai.TaskSummarize
	ladder := ai.TaskLadder(task)
	if len(ladder) != 2 {
		t.Fatalf("%s's ladder is %v; this test needs two rungs", task, ladder)
	}
	first, next := ladder[0], ladder[1]
	profile := ai.ProfileCloudFrontier
	cheap := ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/cheap-1"}
	dear := ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/dear-2"}
	cheapRecord := measuredRecord(task, cheap, profile, aicert.VerdictSupportedDegraded, 0)
	cases := []struct {
		name    string
		tiers   map[ai.Tier]ai.ProviderConfig
		records []aicert.Record
		want    string
	}{
		{
			"a measured fallback",
			map[ai.Tier]ai.ProviderConfig{first: cheap, next: dear},
			[]aicert.Record{cheapRecord, measuredRecord(task, dear, profile, aicert.VerdictCertified, 0)},
			"cheap-1 · " + string(first) + " → dear-2, " + aiCertReady,
		},
		{
			"an unmeasured fallback",
			map[ai.Tier]ai.ProviderConfig{first: cheap, next: dear},
			[]aicert.Record{cheapRecord},
			"cheap-1 · " + string(first) + " → dear-2, not measured on this feature",
		},
		{
			"a fallback binding the same model",
			map[ai.Tier]ai.ProviderConfig{first: cheap, next: cheap},
			[]aicert.Record{cheapRecord},
			"cheap-1 · " + string(first) + " · no separate fallback",
		},
		{
			"no further bound rung",
			map[ai.Tier]ai.ProviderConfig{first: cheap},
			[]aicert.Record{cheapRecord},
			"cheap-1 · " + string(first),
		},
		{
			"runs that broke off",
			map[ai.Tier]ai.ProviderConfig{first: cheap, next: dear},
			[]aicert.Record{measuredRecord(task, cheap, profile, aicert.VerdictNotSupported, 2)},
			"cheap-1 · " + string(first) + " → dear-2, not measured on this feature; 2 of 6 runs broke off and would have gone to dear-2",
		},
		{
			"runs that broke off with no other model",
			map[ai.Tier]ai.ProviderConfig{first: cheap},
			[]aicert.Record{measuredRecord(task, cheap, profile, aicert.VerdictNotSupported, 2)},
			"cheap-1 · " + string(first) + "; 2 of 6 runs broke off, with no other model to take them",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := attributeOneTask(task, profile, tc.tiers, tc.records...).Tasks[0]
			if got := aiCertRouteLine(row); got != tc.want {
				t.Errorf("route line = %q\nwant         %q", got, tc.want)
			}
			if cell := aiCertGradeCell(row); cell != aiCertGrade(row)+"<br><sub>"+tc.want+"</sub>" {
				t.Errorf("grade cell = %q, want the grade with the route line under it", cell)
			}
		})
	}
}

// A Vertex fallback graded by the AI Studio record for its model says so on its
// route line, as a borrowed first rung does in the measurement table.
func TestABorrowedFallbackGradeSaysWhereItWasMeasured(t *testing.T) {
	task := ai.TaskSummarize
	ladder := ai.TaskLadder(task)
	first, next := ladder[0], ladder[1]
	cheap := ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/cheap-1"}
	vertex := ai.ProviderConfig{Provider: "gemini_vertex", Location: "eu", Model: "gemini-3.5-flash"}
	studio := ai.ProviderConfig{Provider: "gemini", Model: vertex.Model}
	row := attributeOneTask(task, ai.ProfileEUHosted, map[ai.Tier]ai.ProviderConfig{first: cheap, next: vertex},
		measuredRecord(task, cheap, ai.ProfileEUHosted, aicert.VerdictCertified, 0),
		measuredRecord(task, studio, ai.ProfileCloudFrontier, aicert.VerdictCertified, 0)).Tasks[0]
	want := "cheap-1 · " + string(first) + " → gemini-3.5-flash, " + aiCertReady + ", measured on `gemini`"
	if got := aiCertRouteLine(row); got != want {
		t.Errorf("route line = %q\nwant         %q", got, want)
	}
}

// A local-only task on a cloud preset is graded from its record like any other
// feature: the router serves it there while the local_only rule is undecided.
func TestALocalOnlyTaskOnACloudPresetIsGradedFromItsRecord(t *testing.T) {
	task := ai.LocalOnlyTasks()[0]
	binding := ai.ProviderConfig{Provider: "openai_compatible", Model: "vendor/m", BaseURL: "https://broker.example/api"}
	tiers := map[ai.Tier]ai.ProviderConfig{}
	for _, tier := range ai.TaskLadder(task) {
		tiers[tier] = binding
	}
	preset := attributeOneTask(task, ai.ProfileCloudFrontier, tiers,
		measuredRecord(task, binding, ai.ProfileCloudFrontier, aicert.VerdictCertified, 0))
	row := preset.Tasks[0]
	if aiCertGrade(row) != aiCertReady || preset.Bands.Certified != 1 {
		t.Errorf("grade = %q with %d certified, want %q from its record", aiCertGrade(row), preset.Bands.Certified, aiCertReady)
	}
	if got := aiCertBottomLine(preset); got != "1 of 1 features ready" {
		t.Errorf("bottom line = %q", got)
	}
}
