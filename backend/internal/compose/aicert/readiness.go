// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Certification readiness: every shipped invocation site paired with the
// records covering it, and the judgement of whether each of those records still
// describes what this build sends.
//
// It lives in the library rather than in the report command because it has two
// renderers — the terminal table `make e2e-ai-report` prints and the generated
// docs/reference/ai-certification.md page — and a second copy of the stale rule
// would let the two disagree about what a band still claims. The renderers own
// their layout; nothing here formats a table.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

// The four states a shipped site's certification can be in. They are separate
// words because they are separate claims: a current record describes what this
// build sends, a partial one is current about every case it measured and has
// not seen the ones added since, a stale one describes prompts it no longer
// sends, and an absent one describes nothing. Collapsing stale into absent
// would hide a lie behind a gap; collapsing absent into a zeroed row would
// print a measurement nobody made.
const (
	StatusCurrent = "current"
	StatusStale   = "stale"
	StatusPartial = "partial"
	StatusAbsent  = "absent"
)

// Unmeasured is what a column shows when there is no record to read it from. A
// dash rather than a zero, because zero accepted runs is a result and "never
// run" is not.
const Unmeasured = "-"

// Census is the shipped surface as readiness reads it: every registered
// invocation site, and how much of each one the case bound to it can cover. The
// two travel together because a site alone no longer answers the second
// question — a case that measures less than its production path says so itself
// — and a reader looking at a site with no record needs the answer that case
// would give.
type Census struct {
	Sites []aitasks.Site
	// Scopes is keyed by "task/variant" (aitasks.Registry.Scopes). A site absent
	// from it declared nothing, and is read at its kind.
	Scopes map[string]string
}

// ScopeFor names the most one site could claim, falling back to its kind when
// its case declares nothing.
func (c Census) ScopeFor(site aitasks.Site) string {
	if scope, declared := c.Scopes[string(site.Task)+"/"+site.Variant]; declared {
		return scope
	}
	return site.CertifiedScope()
}

// Standing is what a record is still right about on one site, and — when it is
// no longer right about everything — why.
//
// The why is carried rather than derived by a reader because the two causes
// want different work. A named scenario that moved prices re-certification at
// that case; a record predating per-scenario stamps prices it at the whole
// task, and can name nothing finer than "something moved".
type Standing struct {
	// Stale is set only by a scenario the record MEASURED whose stamp has since
	// changed. A scenario the record never saw cannot make it wrong about
	// anything, which is the whole difference per-scenario stamps buy.
	Stale bool
	// Measured, Pending and Total count this site's current scenarios against
	// the record's own per-scenario stamps: Measured is how many it scored and
	// still describes, Pending how many it has never seen, and Total how many
	// the site ships today.
	//
	// Measured + Pending is NOT Total: a scenario the record scored and that has
	// since moved is in neither, which is why the denominator is carried rather
	// than added up. A row reading "2/2" beside twenty-one moved scenarios says
	// the site is fully measured, and it is the opposite.
	Measured int
	Pending  int
	Total    int
	// Moved names the scenarios whose stamp changed under this record — the
	// cases a re-run has to cover, and nothing more.
	Moved []string
	// Dropped names scenarios the record measured that the corpus no longer
	// holds. They do not make it stale — it is not wrong about what ships — but
	// they are not coverage either, because nobody can re-run them.
	Dropped []string
	// TaskStampOnly marks a record written before ScenarioRecord.Stamp existed.
	// There is nothing finer to compare than the task stamp it carries, so its
	// staleness names no scenario. Such a record reports no counts, exactly as
	// it did before per-scenario stamps landed.
	TaskStampOnly bool
}

// Reason states why the record is no longer current, in the terms a reader
// acts on. Empty when it still is: a caller prints this beside a `stale` row
// and nowhere else.
func (s Standing) Reason() string {
	if !s.Stale {
		return ""
	}
	var reason string
	switch {
	case s.TaskStampOnly:
		reason = "predates per-scenario stamps: only its task stamp can be compared, and that has moved"
	case len(s.Moved) == 1:
		reason = "scenario " + s.Moved[0] + " — or the prompt this build now builds from it — has changed since the record scored it"
	default:
		reason = fmt.Sprintf("%d scenarios it scored have changed since, or the prompts built from them have: %s",
			len(s.Moved), namesUpTo(s.Moved, movedNamesShown))
	}
	if len(s.Dropped) > 0 {
		reason += fmt.Sprintf(" (it also scored %s, which the corpus no longer holds)",
			namesUpTo(s.Dropped, movedNamesShown))
	}
	return reason
}

// movedNamesShown is how many scenario names a reason spells before it counts
// the rest. A record that lost two cases is re-certified case by case and the
// names are the work order; one that lost twenty is re-certified whole, and
// twenty names in a table cell only cost the reader the sentence around them.
const movedNamesShown = 6

// namesUpTo spells at most n names and says how many it did not.
func namesUpTo(names []string, n int) string {
	if len(names) <= n {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:n], ", "), len(names)-n)
}

// ReadinessRow is one shipped site as readiness reports it: the site itself,
// which is known from the census alone, the record covering it, which may not
// exist, and THAT SITE'S OWN share of it. Certified says which of the two a row
// is — a Record zero value and a genuinely all-zero record are
// indistinguishable otherwise.
//
// The tally is per site rather than per record because a record is written per
// TASK: printing the task's pooled numbers on each of its sites' rows gives
// four identical rows wearing four different labels, and a reader cannot tell
// which site the numbers came from — which is the whole reason the row is the
// site.
type ReadinessRow struct {
	Site aitasks.Site
	// Scope is the most this site could ever claim: what its bound case covers,
	// which is the site's kind only for a case that measures its whole path.
	Scope     string
	Record    Record
	Tally     SiteTally
	Certified bool
	Standing  Standing
}

// Status names which state this row is in.
//
// `partial` sits between current and stale and is the honest word for the
// common case: every scenario this record measured is still current, and the
// corpus has since grown cases it has never seen. Reporting that as stale would
// say the record describes prompts it no longer sends, which is false — and
// would price clearing it at a whole task instead of the new cases.
func (r ReadinessRow) Status() string {
	switch {
	case !r.Certified:
		return StatusAbsent
	case r.Standing.Stale:
		return StatusStale
	case r.Standing.Pending > 0:
		return StatusPartial
	default:
		return StatusCurrent
	}
}

// Coverage is the scenario count behind the status, for a renderer to print
// beside it: a `partial` is only actionable once a reader knows whether it is
// 9 of 10 or 1 of 10.
func (r ReadinessRow) Coverage() string {
	if !r.Certified || r.Standing.Total == 0 {
		return Unmeasured
	}
	return fmt.Sprintf("%d/%d", r.Standing.Measured, r.Standing.Total)
}

// ClaimedScope is how much of the site the row can claim. With a record it is
// what that run folded to; with none it is the most a run could ever cover,
// which is how the agent loop's one-turn limit — and every case that measures
// less than its site's whole path — stays visible while nothing is certified.
func (r ReadinessRow) ClaimedScope() string {
	if !r.Certified {
		return r.Scope
	}
	if r.Record.CertifiedScope == "" {
		return Unmeasured
	}
	return r.Record.CertifiedScope
}

// Binding names the (provider, model, env) this row was measured on — the whole
// of what a band green-lights. Empty for an absent row, which was measured on
// nothing.
func (r ReadinessRow) Binding() string {
	if !r.Certified {
		return ""
	}
	return r.Record.Provider + " · " + r.Record.ServedModel + " · " + r.Record.EnvClass
}

// SiteKey is the site's name as every tree here spells it.
func (r ReadinessRow) SiteKey() string { return string(r.Site.Task) + "/" + r.Site.Variant }

// Readiness pairs every shipped site with the records covering it, and returns
// alongside them the records no shipped site claims.
//
// The sites come from the census rather than from the records, because absence
// is the finding this exists to surface and a missing record cannot name
// itself. taskStamps is the stamp this build computes per task and perScenario
// the same per "task/variant"; both come from CurrentStamps.
//
// A record is written per TASK and covers every site that task ships, so one
// record can produce several rows and a task certified on two bindings gives a
// site two. The site is the unit here — it is what the contract ships and what
// a scenario names — and each row carries only the scenarios that ran on it.
func Readiness(census Census, taskStamps map[string]string, perScenario map[string]map[string]string, records []Record) (rows []ReadinessRow, unclaimed []Record) {
	byTask := map[string][]Record{}
	for _, rec := range records {
		byTask[rec.Task] = append(byTask[rec.Task], rec)
	}

	rows = make([]ReadinessRow, 0, len(census.Sites))
	claimed := map[string]bool{}
	for _, site := range census.Sites {
		task := string(site.Task)
		scope := census.ScopeFor(site)
		measured := false
		for _, rec := range byTask[task] {
			// A record covers a site only if it ran a scenario ON that site. One
			// that did not measured nothing here, and a row built from its pooled
			// numbers would report another site's runs under this name.
			tally, ok := rec.ForSite(site.Variant)
			if !ok {
				continue
			}
			claimed[RecordKey(rec)] = true
			measured = true
			rows = append(rows, ReadinessRow{
				Site: site, Scope: scope, Record: rec, Tally: tally, Certified: true,
				Standing: scenarioStanding(rec, site.Variant, perScenario[task+"/"+site.Variant], taskStamps[task]),
			})
		}
		if !measured {
			rows = append(rows, ReadinessRow{Site: site, Scope: scope})
		}
	}

	for _, rec := range records {
		if !claimed[RecordKey(rec)] {
			unclaimed = append(unclaimed, rec)
		}
	}
	return rows, unclaimed
}

// scenarioStanding judges one record against THIS SITE's scenarios, one by one.
//
// A record predating ScenarioRecord.Stamp carries none, and there is nothing
// finer to compare — so it falls back to the task stamp it does carry and
// reports no counts. That keeps every existing record readable instead of
// declaring the whole tree unmeasured the day per-scenario stamps landed.
//
// current holds only the scenarios this site ships, keyed by name — see
// CurrentStamps, which is where the per-site split is made.
func scenarioStanding(rec Record, variant string, current map[string]string, taskStamp string) Standing {
	scored := map[string]string{}
	for _, sc := range rec.Scenarios {
		if sc.Site != variant {
			continue // another site's scenarios say nothing about this row
		}
		if sc.Stamp == "" {
			return Standing{Stale: rec.PromptVersion != taskStamp, TaskStampOnly: true}
		}
		scored[sc.Scenario] = sc.Stamp
	}
	if len(scored) == 0 {
		return Standing{Stale: rec.PromptVersion != taskStamp, TaskStampOnly: true}
	}
	var standing Standing
	for name, stamp := range scored {
		want, stillInCorpus := current[name]
		switch {
		case !stillInCorpus:
			standing.Dropped = append(standing.Dropped, name)
		case want != stamp:
			standing.Stale = true
			standing.Moved = append(standing.Moved, name)
		default:
			standing.Measured++
		}
	}
	for name := range current {
		if _, everScored := scored[name]; !everScored {
			standing.Pending++
		}
	}
	// The denominator is the site's whole current corpus, so a scenario that
	// moved is counted as unmeasured rather than dropped out of both halves.
	standing.Total = len(current)
	// Named in a stable order: these reach a committed page, where an unordered
	// map would rewrite the file on every regeneration.
	sort.Strings(standing.Moved)
	sort.Strings(standing.Dropped)
	return standing
}

// CurrentStamps is the certification stamp this build computes: per task, the
// value a record's own PromptVersion must equal to still describe what ships,
// and per "task/variant" the stamp of each of that site's scenarios.
//
// The per-site split matters because a row is one site and a task can ship
// several — cold_start ships four — so counting a task's whole corpus against
// one site reports a fully current site as partial and prints a denominator
// belonging to sites it never measured. The task stamp beside it stays per
// task, because that is the fold a legacy record is judged against.
//
// It fails rather than skipping a task it cannot stamp. The stamp covers the
// request each site's own code builds, so a task that cannot be stamped is a
// corpus this build cannot run — and every record for it would otherwise read
// as stale for a reason no report ever states.
func CurrentStamps(ctx context.Context, corpus []Scenario, census *aitasks.Registry) (taskStamps map[string]string, perSite map[string]map[string]string, err error) {
	byTask := map[string][]Scenario{}
	for _, sc := range corpus {
		byTask[sc.Task] = append(byTask[sc.Task], sc)
	}
	taskStamps = make(map[string]string, len(byTask))
	perSite = make(map[string]map[string]string, len(byTask))
	for task, scenarios := range byTask {
		scoped, stampErr := ScenarioStamps(ctx, scenarios, census)
		if stampErr != nil {
			return nil, nil, fmt.Errorf("task %s: %w", task, stampErr)
		}
		for _, sc := range scenarios {
			key := sc.Task + "/" + sc.Site
			if perSite[key] == nil {
				perSite[key] = map[string]string{}
			}
			perSite[key][sc.Name] = scoped[sc.Name]
		}
		taskStamps[task] = FoldScenarioStamps(scoped)
	}
	return taskStamps, perSite, nil
}

// RecordKey identifies one record the way its own file path does — the four
// fields that make it a distinct measurement.
func RecordKey(rec Record) string {
	return rec.Task + "/" + rec.Provider + "/" + rec.ServedModel + "/" + rec.EnvClass
}
