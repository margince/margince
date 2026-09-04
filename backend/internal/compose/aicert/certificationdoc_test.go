// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// The certification document: what docs/reference/ai-certification.json holds,
// and what docs/reference/ai-certification.md is rendered from.
//
// The page reads the document rather than the trees for the reason mcp-info.md
// reads mcp-info.json: a reader who wants to ask a question this page does not
// answer — which model is fastest at a band we accept, what a provider costs us
// in stale records — gets the same numbers the page shows, and cannot be told
// something the page contradicts. Two renderers over one document can disagree
// about layout and about nothing else.
//
// A number this document leaves out is a number no analysis can recover, so it
// carries the measurements whole and leaves the judging to its readers. It
// holds no timestamp: a generated file that changes when nothing else did makes
// a drift gate fail for the passage of time, and teaches everyone to regenerate
// without reading.

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

// aiCertDoc is the whole certification surface, in the order the page renders
// it: totals, then one row per binding, then every shipped site, then the
// records no site claims.
type aiCertDoc struct {
	Totals    aiCertTotals      `json:"totals"`
	Bindings  []aiCertBinding   `json:"bindings"`
	Sites     []aiCertSite      `json:"sites"`
	Unclaimed []aiCertUnclaimed `json:"unclaimed_records"`
}

type aiCertTotals struct {
	Sites            int `json:"sites"`
	SitesBestCurrent int `json:"sites_best_current"`
	SitesBestPartial int `json:"sites_best_partial"`
	SitesBestStale   int `json:"sites_best_stale"`
	SitesAbsent      int `json:"sites_absent"`
	Scenarios        int `json:"scenarios"`
	Records          int `json:"records"`
	Bindings         int `json:"bindings"`
}

// aiCertBindingRef is the whole of what a band speaks for. It is a struct
// rather than the page's "provider · model · env" string because a reader
// filtering by provider should not have to split one.
type aiCertBindingRef struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Env      string `json:"env"`
}

// label is the binding as the page spells it.
func (b aiCertBindingRef) label() string { return b.Provider + " · " + b.Model + " · " + b.Env }

// aiCertBinding is one binding folded over every site it measured.
type aiCertBinding struct {
	Binding      aiCertBindingRef `json:"binding"`
	Sites        int              `json:"sites"`
	SitesCurrent int              `json:"sites_current"`
	SitesPartial int              `json:"sites_partial"`
	SitesStale   int              `json:"sites_stale"`
	Runs         int              `json:"runs"`
	Passed       int              `json:"passed"`
	Reliability  *float64         `json:"reliability"`
	// SlowestP95MS is the highest p95 of any record folded here, never a mean of
	// them: percentiles do not average, and a mean of p95s is a figure nothing
	// measured.
	SlowestP95MS *int64      `json:"slowest_p95_ms"`
	Bands        aiCertBands `json:"bands"`
}

type aiCertBands struct {
	Certified         int `json:"certified"`
	SupportedDegraded int `json:"supported_degraded"`
	NotSupported      int `json:"not_supported"`
}

// aiCertSite is one shipped invocation site: what it is scored against, how
// every binding did on it, and which of those a reader should run today.
type aiCertSite struct {
	Task string `json:"task"`
	Site string `json:"site"`
	Key  string `json:"key"`
	// Scope is the most a run of this site can claim.
	Scope string `json:"scope"`
	// MissingContextScopes is the company context production prepends that no
	// certification run was served, because this lane has no database to
	// assemble it from. Empty for a task whose contract prepends none.
	MissingContextScopes []string `json:"missing_context_scopes"`
	BestState            string   `json:"best_state"`
	// Recommended is the strongest record that still describes what ships, or
	// null when every record here is stale or absent. Its band decides whether
	// it is a model to run or a gap to fill; this names it, and judges nothing.
	Recommended *aiCertPick      `json:"recommended"`
	Scenarios   []aiCertScenario `json:"scenarios"`
	Records     []aiCertRecord   `json:"records"`
}

type aiCertPick struct {
	Binding     aiCertBindingRef `json:"binding"`
	Band        string           `json:"band"`
	Reliability *float64         `json:"reliability"`
	State       string           `json:"state"`
}

type aiCertScenario struct {
	Name    string `json:"name"`
	Expects string `json:"expects"`
	// File is the case, as a path from the repository root, so a reader of the
	// JSON can open it without knowing where this page sits.
	File string `json:"file"`
}

// aiCertRecord is one binding's measurement of one site. The counts are what
// the site's own validator reported, not a pass/fail split.
type aiCertRecord struct {
	Binding aiCertBindingRef `json:"binding"`
	State   string           `json:"state"`
	Band    string           `json:"band"`
	// ScenariosMeasured is null for a record written before per-scenario stamps,
	// which can only be judged whole. Zero would claim it measured nothing.
	ScenariosMeasured *int           `json:"scenarios_measured"`
	ScenariosTotal    int            `json:"scenarios_total"`
	Runs              int            `json:"runs"`
	Passed            int            `json:"passed"`
	Reliability       *float64       `json:"reliability"`
	LatencyP50MS      *int64         `json:"latency_p50_ms"`
	LatencyP95MS      *int64         `json:"latency_p95_ms"`
	Reported          aiCertOutcomes `json:"reported"`
	StaleReason       string         `json:"stale_reason"`
}

type aiCertOutcomes struct {
	Accepted    int `json:"accepted"`
	WrongAnswer int `json:"wrong_answer"`
	Invalid     int `json:"invalid"`
	Abstained   int `json:"abstained"`
}

type aiCertUnclaimed struct {
	Task    string           `json:"task"`
	Binding aiCertBindingRef `json:"binding"`
	Reason  string           `json:"reason"`
}

// unclaimedReason is why a committed record reaches no row. The page and the
// document both render it, so it is a constant rather than a sentence written
// into each of them, which is how the two would come to word it differently.
const unclaimedReason = "it names no scenario of any site its task ships, so no row above can attribute it"

// buildAICertDoc folds the three trees into the document both files are written
// from. The site order is the page's — tasks alphabetically, each task's sites
// as the census yielded them — so a reader diffing two regenerations sees what
// moved rather than where a map walk landed.
func buildAICertDoc(rows []aicert.ReadinessRow, unclaimed []aicert.Record,
	corpus []aicert.Scenario, records []aicert.Record,
) aiCertDoc {
	scenarios := scenariosBySite(corpus)
	tasks, byTask := aiCertTaskGroups(rows)
	doc := aiCertDoc{Bindings: foldAICertBindings(rows)}
	for _, task := range tasks {
		for _, siteKey := range siteKeysOf(byTask[task]) {
			doc.Sites = append(doc.Sites, buildAICertSite(siteKey, byTask[task], scenarios[siteKey]))
		}
	}
	for _, rec := range unclaimed {
		doc.Unclaimed = append(doc.Unclaimed, aiCertUnclaimed{
			Task: rec.Task, Binding: bindingRefOf(rec), Reason: unclaimedReason,
		})
	}
	doc.Totals = aiCertTotals{
		Sites: len(doc.Sites), Scenarios: len(corpus), Records: len(records), Bindings: len(doc.Bindings),
	}
	for _, site := range doc.Sites {
		switch site.BestState {
		case aicert.StatusCurrent:
			doc.Totals.SitesBestCurrent++
		case aicert.StatusPartial:
			doc.Totals.SitesBestPartial++
		case aicert.StatusStale:
			doc.Totals.SitesBestStale++
		default:
			doc.Totals.SitesAbsent++
		}
	}
	return doc
}

func buildAICertSite(siteKey string, taskRows []aicert.ReadinessRow, cases []aicert.Scenario) aiCertSite {
	var mine []aicert.ReadinessRow
	for _, row := range taskRows {
		if row.SiteKey() == siteKey {
			mine = append(mine, row)
		}
	}
	site := aiCertSite{
		Task: string(mine[0].Site.Task), Site: mine[0].Site.Variant, Key: siteKey,
		Scope: mine[0].Scope, MissingContextScopes: []string{},
		BestState: bestStateOf(mine), Scenarios: []aiCertScenario{}, Records: []aiCertRecord{},
	}
	for _, sc := range cases {
		site.Scenarios = append(site.Scenarios, aiCertScenario{
			Name: sc.Name, Expects: sc.Expect.Outcome, File: repoPathOf(sc),
		})
	}
	for _, row := range mine {
		if !row.Certified {
			continue
		}
		if !row.Record.ContextApplied && len(row.Record.ContextScopes) > 0 {
			site.MissingContextScopes = row.Record.ContextScopes
		}
		site.Records = append(site.Records, buildAICertRecord(row))
	}
	site.Recommended = pickAICertRecord(site.Records)
	return site
}

func buildAICertRecord(row aicert.ReadinessRow) aiCertRecord {
	rec := aiCertRecord{
		Binding: bindingRefOf(row.Record), State: row.Status(), Band: row.Tally.Verdict,
		ScenariosTotal: row.Standing.Total, Runs: row.Tally.Runs, Passed: row.Tally.Passed,
		Reliability:  ratio(row.Tally.Passed, row.Tally.Runs),
		LatencyP50MS: measuredMS(row.Record.LatencyP50), LatencyP95MS: measuredMS(row.Record.LatencyP95),
		Reported: aiCertOutcomes{
			Accepted: row.Tally.ReportedAccepted, WrongAnswer: row.Tally.ReportedWrongAnswer,
			Invalid: row.Tally.ReportedInvalid, Abstained: row.Tally.ReportedAbstained,
		},
		StaleReason: row.Standing.Reason(),
	}
	if !row.Standing.TaskStampOnly && row.Standing.Total > 0 {
		measured := row.Standing.Measured
		rec.ScenariosMeasured = &measured
	}
	return rec
}

// pickAICertRecord is the strongest record that still describes what ships.
// Only a current or partial record is eligible — a stale one is a measurement
// of a product that no longer exists — and among those the order is band, then
// reliability, then speed: a band is the judgement a whole run reached about
// shipping, and a fast wrong answer is not a cheaper one.
func pickAICertRecord(records []aiCertRecord) *aiCertPick {
	var best *aiCertRecord
	for i, rec := range records {
		if rec.State != aicert.StatusCurrent && rec.State != aicert.StatusPartial {
			continue
		}
		if best == nil || beatsAICertRecord(rec, *best) {
			best = &records[i]
		}
	}
	if best == nil {
		return nil
	}
	return &aiCertPick{
		Binding: best.Binding, Band: best.Band, Reliability: best.Reliability, State: best.State,
	}
}

func beatsAICertRecord(rec, held aiCertRecord) bool {
	if bandRank(rec.Band) != bandRank(held.Band) {
		return bandRank(rec.Band) > bandRank(held.Band)
	}
	if value(rec.Reliability) != value(held.Reliability) {
		return value(rec.Reliability) > value(held.Reliability)
	}
	if slowness(rec.LatencyP95MS) != slowness(held.LatencyP95MS) {
		return slowness(rec.LatencyP95MS) < slowness(held.LatencyP95MS)
	}
	return rec.Binding.label() < held.Binding.label()
}

func bandRank(band string) int {
	switch band {
	case aicert.VerdictCertified:
		return 2
	case aicert.VerdictSupportedDegraded:
		return 1
	default:
		return 0
	}
}

// bestStateOf is the strongest state any of a site's bindings reached, because
// "is this site certified" is asked of the site and answered by whichever
// binding answers it best.
func bestStateOf(rows []aicert.ReadinessRow) string {
	rank := map[string]int{
		aicert.StatusAbsent: 0, aicert.StatusStale: 1, aicert.StatusPartial: 2, aicert.StatusCurrent: 3,
	}
	best := aicert.StatusAbsent
	for _, row := range rows {
		if rank[row.Status()] > rank[best] {
			best = row.Status()
		}
	}
	return best
}

// foldAICertBindings groups every certified row by binding, ordered so the
// document is a function of the trees and not of a map walk.
func foldAICertBindings(rows []aicert.ReadinessRow) []aiCertBinding {
	byKey := map[string]*aiCertBinding{}
	sites := map[string]map[string]bool{}
	var order []string
	for _, row := range rows {
		if !row.Certified {
			continue
		}
		ref := bindingRefOf(row.Record)
		key := ref.label()
		fold := byKey[key]
		if fold == nil {
			fold = &aiCertBinding{Binding: ref}
			byKey[key], sites[key] = fold, map[string]bool{}
			order = append(order, key)
		}
		sites[key][row.SiteKey()] = true
		countAICertSiteInto(fold, row)
	}
	out := make([]aiCertBinding, 0, len(order))
	sort.Strings(order)
	for _, key := range order {
		fold := byKey[key]
		fold.Sites = len(sites[key])
		fold.Reliability = ratio(fold.Passed, fold.Runs)
		out = append(out, *fold)
	}
	return out
}

func countAICertSiteInto(fold *aiCertBinding, row aicert.ReadinessRow) {
	switch row.Status() {
	case aicert.StatusCurrent:
		fold.SitesCurrent++
	case aicert.StatusPartial:
		fold.SitesPartial++
	case aicert.StatusStale:
		fold.SitesStale++
	}
	switch row.Tally.Verdict {
	case aicert.VerdictCertified:
		fold.Bands.Certified++
	case aicert.VerdictSupportedDegraded:
		fold.Bands.SupportedDegraded++
	case aicert.VerdictNotSupported:
		fold.Bands.NotSupported++
	}
	fold.Runs += row.Tally.Runs
	fold.Passed += row.Tally.Passed
	if p95 := row.Record.LatencyP95; p95 > 0 && (fold.SlowestP95MS == nil || p95 > *fold.SlowestP95MS) {
		fold.SlowestP95MS = &p95
	}
}

func bindingRefOf(rec aicert.Record) aiCertBindingRef {
	return aiCertBindingRef{Provider: rec.Provider, Model: rec.ServedModel, Env: rec.EnvClass}
}

// repoPathOf names a scenario file from the repository root. The loader reads
// it relative to this package, which is a path only this test can resolve.
func repoPathOf(sc aicert.Scenario) string {
	return "backend/internal/compose/aicert/" + filepath.ToSlash(sc.Path)
}

// ratio is a pass rate, or null when there is nothing to divide. Zero runs is
// not a reliability of zero, and a JSON zero would be read as one.
func ratio(passed, runs int) *float64 {
	if runs == 0 {
		return nil
	}
	rate := float64(passed) / float64(runs)
	return &rate
}

// measuredMS is a latency, or null when the record kept none.
func measuredMS(ms int64) *int64 {
	if ms <= 0 {
		return nil
	}
	return &ms
}

func value(rate *float64) float64 {
	if rate == nil {
		return 0
	}
	return *rate
}

// slowness orders records by p95 with an unmeasured one last, so a record that
// timed nothing never wins the recommendation for being infinitely fast.
func slowness(ms *int64) int64 {
	if ms == nil {
		return int64(^uint64(0) >> 1)
	}
	return *ms
}

// marshalAICertDoc encodes the document as the committed sibling file: indented
// so a diff is readable line by line, and with HTML escaping off so a scenario
// name reads as itself.
func marshalAICertDoc(doc aiCertDoc) ([]byte, error) {
	var encoded strings.Builder
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return nil, err
	}
	return []byte(encoded.String()), nil
}
