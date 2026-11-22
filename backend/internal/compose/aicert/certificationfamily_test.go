// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// The family view of the certification document: the records folded by model
// family, for a reader who asks "can I run Margince on Gemini" before choosing
// a preset. It re-reads the same site records the preset view reads, so the two
// can differ in layout and never in a grade.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// The answers a family can earn. The first three read the family's best model
// on each feature it was measured on; the last says the testing is too thin to
// answer at all, rather than forcing a colour it has not earned.
const (
	familyYes       = "yes"
	familyMostly    = "mostly"
	familyNotYet    = "not_yet"
	familyNotEnough = "not_enough_tested"
)

const (
	// familyMostlyPercent is how many of the measured features must be at least
	// usable with care for a family to read "mostly". Below it, a buyer running
	// the family would meet a feature that does not work more often than not
	// enough to call it trustworthy.
	familyMostlyPercent = 80
	// familyCoverageDivisor makes the coverage floor half the shipped features:
	// a family measured on fewer says too little to grade.
	familyCoverageDivisor = 2
)

// aiCertFamily is one model family across every feature Margince ships.
type aiCertFamily struct {
	Name   string `json:"name"`
	Answer string `json:"answer"`
	// SitesShipped counts the features the product runs, so a reader sees how much
	// of it this family was measured on.
	SitesShipped   int `json:"sites_shipped"`
	SitesMeasured  int `json:"sites_measured"`
	Ready          int `json:"ready"`
	UsableWithCare int `json:"usable_with_care"`
	NotReliableYet int `json:"not_reliable_yet"`
	// TooFewRuns counts places right on every case with too few tries to grade
	// alone: measured, and neither ready nor weak.
	TooFewRuns     int `json:"too_few_runs"`
	RecheckPending int `json:"recheck_pending"`
	// Judges are the models that scored this family's answers, so a reader can
	// see when two families' quality grades come from different judges.
	Judges []string `json:"judges"`
	// Models are the family's bindings folded over only the features a preset
	// routes to them, so a model no preset reaches is not in any family.
	Models []aiCertBinding `json:"models"`
	// Sites are keyed the way a record is, by feature, so a reader can also ask
	// which model in a family serves one feature best.
	Sites []aiCertFamilySite `json:"sites"`
}

// aiCertFamilySite is the family's best result on one feature.
type aiCertFamilySite struct {
	Key     string           `json:"key"`
	Task    string           `json:"task"`
	Best    aiCertBindingRef `json:"best"`
	Band    string           `json:"band"`
	State   string           `json:"state"`
	Runs    int              `json:"runs"`
	Passed  int              `json:"passed"`
	Records int              `json:"records"`
	Judge   string           `json:"judge,omitempty"`
}

// buildAICertFamilies folds the document's records by model family. A family is
// graded on its best model per feature: the model a buyer picking that family
// would run. So a weak model in the family cannot hide a strong one. Only a
// route a preset takes and a record grades counts. A retired or one-off binding
// is a model no buyer is given. A binding sent with settings no record measured
// has no grade to list.
func buildAICertFamilies(doc aiCertDoc, rows []aicert.ReadinessRow) []aiCertFamily {
	measured := aiCertMeasuredRoutes(doc.Presets)
	byName := map[string]*aiCertFamily{}
	for _, site := range doc.Sites {
		best := map[string]aiCertRecord{}
		count := map[string]int{}
		for _, rec := range eligibleFamilyRecords(site, measured) {
			name := aicert.ModelFamily(rec.Binding.Model)
			count[name]++
			if held, ok := best[name]; !ok || beatsAICertRecord(rec, held) {
				best[name] = rec
			}
		}
		for name, rec := range best {
			fam := byName[name]
			if fam == nil {
				fam = &aiCertFamily{Name: name, SitesShipped: len(doc.Sites)}
				byName[name] = fam
			}
			fam.Sites = append(fam.Sites, aiCertFamilySite{
				Key: site.Key, Task: site.Task, Best: rec.Binding, Band: rec.Band, State: rec.State,
				Runs: rec.Runs, Passed: rec.Passed, Records: count[name], Judge: rec.Judge,
			})
		}
	}
	var routedRows []aicert.ReadinessRow
	for _, row := range rows {
		if measured[aiCertRouteKey(string(row.Site.Task), bindingRefOf(row.Record))] {
			routedRows = append(routedRows, row)
		}
	}
	for _, b := range foldAICertBindings(routedRows) {
		if fam := byName[aicert.ModelFamily(b.Binding.Model)]; fam != nil {
			fam.Models = append(fam.Models, b)
		}
	}
	out := make([]aiCertFamily, 0, len(byName))
	for _, fam := range byName {
		countAICertFamily(fam)
		out = append(out, *fam)
	}
	sort.Slice(out, func(i, j int) bool { return familyOrder(out[i], out[j]) })
	return out
}

// eligibleFamilyRecords keeps the site's records that grade a route some preset
// takes, so a record no buyer's preset reaches cannot lift or sink a family.
func eligibleFamilyRecords(site aiCertSite, routed map[string]bool) []aiCertRecord {
	var eligible []aiCertRecord
	for _, rec := range site.Records {
		if routed[aiCertRouteKey(site.Task, rec.Binding)] {
			eligible = append(eligible, rec)
		}
	}
	return eligible
}

// aiCertMeasuredRoutes marks the routes a preset row actually grades: a rung is
// routed but unmeasured when its record ran the sites at other thinking levels
// (measuredOn), and that record must not grade the family the row says nothing of.
func aiCertMeasuredRoutes(presets []aiCertPreset) map[string]bool {
	measured := map[string]bool{}
	for _, p := range presets {
		for _, row := range p.Tasks {
			if row.Band != "" {
				measured[aiCertRouteKey(row.Task, row.Model)] = true
			}
			if row.Fallback != nil && row.Fallback.Band != "" {
				measured[aiCertRouteKey(row.Task, row.Fallback.Model)] = true
			}
		}
	}
	return measured
}

func aiCertRouteKey(task string, binding aiCertBindingRef) string {
	return task + "\x00" + binding.label()
}

func countAICertFamily(fam *aiCertFamily) {
	fam.SitesMeasured = len(fam.Sites)
	judges := map[string]bool{}
	for _, s := range fam.Sites {
		switch s.Band {
		case aicert.VerdictCertified:
			fam.Ready++
		case aicert.SiteTooFewRuns:
			fam.TooFewRuns++
		case aicert.VerdictSupportedDegraded:
			fam.UsableWithCare++
		default:
			fam.NotReliableYet++
		}
		if s.State == aicert.StatusStale {
			fam.RecheckPending++
		}
		if s.Judge != "" && !judges[s.Judge] {
			judges[s.Judge] = true
			fam.Judges = append(fam.Judges, s.Judge)
		}
	}
	sort.Strings(fam.Judges)
	fam.Answer = familyAnswer(fam)
}

// familyAnswer grades a family, and both halves of the page state its rule.
// Under half the features covered reads not enough tested. Every graded one
// ready reads yes, familyMostlyPercent usable reads mostly, else not yet.
// A place with too few tries to grade alone counts toward coverage and no grade.
func familyAnswer(fam *aiCertFamily) string {
	graded := fam.SitesMeasured - fam.TooFewRuns
	switch {
	case fam.SitesMeasured*familyCoverageDivisor < fam.SitesShipped:
		return familyNotEnough
	case fam.Ready == graded:
		return familyYes
	case (fam.Ready+fam.UsableWithCare)*100 >= graded*familyMostlyPercent:
		return familyMostly
	default:
		return familyNotYet
	}
}

// familyOrder puts the families a buyer can act on first: best answer, then the
// widest measurement, then the name so the order never rests on a map walk.
func familyOrder(a, b aiCertFamily) bool {
	if rank := familyAnswerRank(a.Answer) - familyAnswerRank(b.Answer); rank != 0 {
		return rank > 0
	}
	if a.SitesMeasured != b.SitesMeasured {
		return a.SitesMeasured > b.SitesMeasured
	}
	return a.Name < b.Name
}

func familyAnswerRank(answer string) int {
	switch answer {
	case familyYes:
		return 3
	case familyMostly:
		return 2
	case familyNotYet:
		return 1
	default:
		return 0
	}
}

func familyAnswerWords(answer string) string {
	switch answer {
	case familyYes:
		return "🟢 Yes"
	case familyMostly:
		return "🟡 Mostly"
	case familyNotYet:
		return "🔴 Not yet"
	default:
		return "⚪ Not enough tested yet"
	}
}

// familyPlainWords is the sentence for a reader without the vocabulary.
// It is built from the counts, the same ones the answer beside it reads.
func familyPlainWords(fam aiCertFamily) string {
	if fam.Answer == familyNotEnough {
		return fmt.Sprintf("Tested in only %d of the %d places. Ask before relying on it.",
			fam.SitesMeasured, fam.SitesShipped)
	}
	words := fmt.Sprintf("Ready in %d of the %d places we tested", fam.Ready, fam.SitesMeasured)
	if fam.UsableWithCare > 0 {
		words += fmt.Sprintf("; %d more work if someone looks over the result", fam.UsableWithCare)
	}
	if fam.NotReliableYet > 0 {
		words += fmt.Sprintf("; %d not reliable yet", fam.NotReliableYet)
	}
	if fam.TooFewRuns > 0 {
		words += fmt.Sprintf("; %d more right every time, on too few tries to grade alone", fam.TooFewRuns)
	}
	return words + "."
}

// aiCertShortModel is a served model without its broker's publisher prefix,
// which is how a reader of the page's top half knows the model.
func aiCertShortModel(model string) string {
	return model[strings.LastIndex(model, "/")+1:]
}

// familyModelNames names the models behind a family's answer, without the
// broker prefix or the environment, because a family name alone misleads: "GPT"
// here is an open-weight model, not the vendor's flagship.
func familyModelNames(fam aiCertFamily) string {
	var names []string
	seen := map[string]bool{}
	for _, m := range fam.Models {
		name := aiCertShortModel(m.Binding.Model)
		if !seen[name] {
			seen[name] = true
			names = append(names, "`"+name+"`")
		}
	}
	return strings.Join(names, ", ")
}

// writeAICertFamilySummary is the page's first answer: nothing in it needs the
// words preset, tier or provider.
func writeAICertFamilySummary(page *strings.Builder, families []aiCertFamily) {
	page.WriteString("## Which model family should I use?\n\n")
	page.WriteString("*\"I want to run Margince on Gemini. Can I trust it?\"* This is the answer for each\n")
	page.WriteString("family of AI models, across everything Margince AI does. A *place* is one spot in the\n")
	page.WriteString("product where a feature calls a model; one feature can call from several places. To start\n")
	page.WriteString("from a ready-made setup instead, go to [Can I use this model setup?](#can-i-use-this-model-setup).\n\n")
	page.WriteString("| Family | Models we tested | Can I trust it? | In plain words | Tries right | Quality scored by |\n|---|---|---|---|---|---|\n")
	for _, fam := range families {
		fmt.Fprintf(page, "| %s | %s | %s | %s | %s | %s |\n", fam.Name, familyModelNames(fam), familyAnswerWords(fam.Answer),
			familyPlainWords(fam), familyTriesRight(fam), familyJudges(fam))
	}
	page.WriteString("\n")
	if familiesDisagreeOnJudges(families) {
		page.WriteString("**The families above were scored by different judges.** A judge is the second model that scores how good an\n" +
			"answer is, and two judges do not score on one scale. Until one judge has scored them all, compare families on\n" +
			"*Tries right*: how often the answer passed its mechanical check, which no judge decides.\n\n")
	}
	fmt.Fprintf(page, "A family rated **Yes** is ready in every place we tested. One rated **Mostly** has at least %d in every 100 "+
		"tested places ready or working with a check, and one rated **Not yet** has fewer. We say **Not enough tested yet** "+
		"when we tested it in under half of the places. A family is judged on its best "+
		"model for each place, and only on the models a model setup uses, each in the places that setup "+
		"sends to it.\n\n", familyMostlyPercent)
}

// familyTriesRight is the family's pooled mechanical pass count over its best
// model per place, which no judge's scale moves.
func familyTriesRight(fam aiCertFamily) string {
	runs, passed := 0, 0
	for _, s := range fam.Sites {
		runs += s.Runs
		passed += s.Passed
	}
	return fmt.Sprintf("%d of %d", passed, runs)
}

func familyJudges(fam aiCertFamily) string {
	if len(fam.Judges) == 0 {
		return "checked mechanically"
	}
	names := make([]string, len(fam.Judges))
	for i, judge := range fam.Judges {
		names[i] = "`" + judge + "`"
	}
	return strings.Join(names, ", ")
}

// familiesDisagreeOnJudges says two families' quality grades rest on different
// judges, or one family's on more than one.
func familiesDisagreeOnJudges(families []aiCertFamily) bool {
	seen := map[string]bool{}
	for _, fam := range families {
		for _, judge := range fam.Judges {
			seen[judge] = true
		}
	}
	return len(seen) > 1
}

// writeAICertFamilyDetail is the engineers' half: every model in each family and
// its best result on every feature it was measured on.
func writeAICertFamilyDetail(page *strings.Builder, families []aiCertFamily) {
	writeAICertFolded(page, "See the detail behind each family", func(page *strings.Builder) {
		page.WriteString("A family counts a model only in the places some preset sends it, as the answering\n")
		page.WriteString("model or its fallback. A record no preset reaches is left out here and still listed\n")
		page.WriteString("under [Certification by provider and model](#certification-by-provider-and-model) and its site.\n\n")
		for _, fam := range families {
			writeAICertFamilySection(page, fam)
		}
	})
}

func writeAICertFamilySection(page *strings.Builder, fam aiCertFamily) {
	fmt.Fprintf(page, "### %s\n\n", fam.Name)
	fmt.Fprintf(page, "%s: %s\n\n", familyAnswerWords(fam.Answer), familyPlainWords(fam))
	if fam.RecheckPending > 0 {
		fmt.Fprintf(page, "%d of these results were measured on an older version of the product and are re-check pending.\n\n",
			fam.RecheckPending)
	}
	page.WriteString("| Model | Where it ran | Places tested | ✅ | ⚠️ | ❌ | Tries right |\n|---|---|---:|---:|---:|---:|---|\n")
	for _, m := range fam.Models {
		fmt.Fprintf(page, "| `%s` | `%s` | %d | %d | %d | %d | %s |\n", m.Binding.Model, m.Binding.Env,
			m.Sites, m.Bands.Certified, m.Bands.SupportedDegraded, m.Bands.NotSupported, aicert.ReliabilityCell(m.Passed, m.Runs))
	}
	page.WriteString("\n<details>\n<summary>Best model for each place</summary>\n\n")
	page.WriteString("| Place | Best model in the family | Grade | Right in | Quality scored by |\n|---|---|---|---|---|\n")
	for _, s := range fam.Sites {
		fmt.Fprintf(page, "| [`%s`](#%s) | `%s` | %s%s | %s | %s |\n", s.Key, aiCertSiteAnchor(s.Key),
			s.Best.label(), bandGrade(s.Band), staleMark(s.State), aiCertTriesWords(s.Runs, s.Passed), judgeCell(s.Judge))
	}
	page.WriteString("\n</details>\n\n")
}

func bandGrade(band string) string {
	switch band {
	case aicert.VerdictCertified:
		return aiCertReady
	case aicert.SiteTooFewRuns:
		return aiCertTooFew
	case aicert.VerdictSupportedDegraded:
		return aiCertCare
	default:
		return aiCertNotYet
	}
}

func staleMark(state string) string {
	if state == aicert.StatusStale {
		return " · re-check pending"
	}
	return ""
}

// assertAICertFamiliesCoverEveryRecord holds the fold to the trees: a builder
// that dropped a record would still match the committed page rendered by the
// same builder, so every routed record of every site must land in exactly one
// family, every binding with one in exactly one family's model table, and a
// binding without one in none.
func assertAICertFamiliesCoverEveryRecord(t *testing.T, doc aiCertDoc, page string) {
	t.Helper()
	rows := map[string]map[string]aiCertFamilySite{}
	for _, fam := range doc.Families {
		rows[fam.Name] = map[string]aiCertFamilySite{}
		for _, s := range fam.Sites {
			rows[fam.Name][s.Key] = s
		}
		if !strings.Contains(page, "| "+fam.Name+" | "+familyModelNames(fam)+" | "+familyAnswerWords(fam.Answer)+" |") {
			t.Errorf("family %s has no row in the summary table", fam.Name)
		}
	}
	measured := aiCertMeasuredRoutes(doc.Presets)
	counted := map[string]bool{}
	for _, site := range doc.Sites {
		perFamily := map[string]int{}
		for _, rec := range eligibleFamilyRecords(site, measured) {
			perFamily[aicert.ModelFamily(rec.Binding.Model)]++
			counted[rec.Binding.label()] = true
		}
		for name, n := range perFamily {
			if got := rows[name][site.Key].Records; got != n {
				t.Errorf("site %s: family %s holds %d of its %d records", site.Key, name, got, n)
			}
		}
	}
	seen := map[string]int{}
	for _, fam := range doc.Families {
		for _, m := range fam.Models {
			seen[m.Binding.label()]++
		}
	}
	for _, b := range doc.Bindings {
		want := 0
		if counted[b.Binding.label()] {
			want = 1
		}
		if seen[b.Binding.label()] != want {
			t.Errorf("binding %s is in %d families' model tables, want %d", b.Binding.label(), seen[b.Binding.label()], want)
		}
	}
}

func TestFamilyAnswerNeedsCoverageBeforeItGrades(t *testing.T) {
	cases := []struct {
		name string
		fam  aiCertFamily
		want string
	}{
		{"every measured feature ready", aiCertFamily{SitesShipped: 10, SitesMeasured: 6, Ready: 6}, familyYes},
		{"eight in ten usable", aiCertFamily{SitesShipped: 10, SitesMeasured: 10, Ready: 5, UsableWithCare: 3, NotReliableYet: 2}, familyMostly},
		{"seven in ten usable", aiCertFamily{SitesShipped: 10, SitesMeasured: 10, Ready: 5, UsableWithCare: 2, NotReliableYet: 3}, familyNotYet},
		{"under half measured", aiCertFamily{SitesShipped: 10, SitesMeasured: 4, Ready: 4}, familyNotEnough},
		{"exactly half measured", aiCertFamily{SitesShipped: 10, SitesMeasured: 5, Ready: 5}, familyYes},
	}
	for _, tc := range cases {
		if got := familyAnswer(&tc.fam); got != tc.want {
			t.Errorf("%s: answer %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A family is graded only on the models a preset routes a feature to: a record
// for a binding no preset reaches — a retired model, a one-off run under another
// profile — neither counts toward the family's answer nor lists its model.
func TestAFamilyCountsOnlyTheBindingsAPresetRoutes(t *testing.T) {
	task := ai.TaskSummarize
	profile := ai.ProfileEUHosted
	bound := ai.ProviderConfig{Provider: "openai_compatible", Model: "mistralai/ministral-14b-2512"}
	retired := ai.ProviderConfig{Provider: "openai_compatible", Model: "mistralai/mistral-large-2512"}
	records := []aicert.Record{
		measuredRecord(task, bound, profile, aicert.VerdictCertified, 0),
		measuredRecord(task, retired, profile, aicert.VerdictNotSupported, 0),
		measuredRecord(task, bound, ai.ProfileCloudFrontier, aicert.VerdictNotSupported, 0),
	}
	tiers := map[ai.Tier]ai.ProviderConfig{}
	for _, tier := range ai.TaskLadder(task) {
		tiers[tier] = bound
	}
	preset := attributeOneTask(task, profile, tiers, records...)
	doc := aiCertDoc{Presets: []aiCertPreset{preset}, Sites: []aiCertSite{{Task: string(task), Key: string(task) + "/brief"}}}
	var rows []aicert.ReadinessRow
	for _, rec := range records {
		doc.Sites[0].Records = append(doc.Sites[0].Records, aiCertRecord{
			Binding: bindingRefOf(rec), State: aicert.StatusCurrent, Band: rec.Verdict, Runs: rec.Runs, Passed: rec.Passed,
		})
		rows = append(rows, aicert.ReadinessRow{
			Site: aitasks.Site{Task: task, Variant: "brief"}, Record: rec, Certified: true,
			Tally: aicert.SiteTally{Verdict: rec.Verdict, Runs: rec.Runs, Passed: rec.Passed},
		})
	}
	families := buildAICertFamilies(doc, rows)
	if len(families) != 1 {
		t.Fatalf("families = %+v, want Mistral alone", families)
	}
	fam := families[0]
	if fam.Ready != 1 || fam.NotReliableYet != 0 || fam.Sites[0].Records != 1 {
		t.Errorf("Mistral counts ready %d, not reliable %d over %d record(s); want only the routed record, ready",
			fam.Ready, fam.NotReliableYet, fam.Sites[0].Records)
	}
	if len(fam.Models) != 1 || fam.Models[0].Binding.label() != bindingRefOf(records[0]).label() {
		t.Errorf("Mistral lists %+v, want only the routed binding", fam.Models)
	}
	if got := familyModelNames(fam); got != "`ministral-14b-2512`" {
		t.Errorf("models we tested = %s, want only the routed model", got)
	}
}

// A record at a routed binding that ran the sites at other thinking levels is
// one the preset row calls unmeasured, so it grades no family either.
func TestAFamilyIgnoresARecordThePresetCallsUnmeasured(t *testing.T) {
	task := ai.TaskSummarize
	profile := ai.ProfileEUHosted
	bound := ai.ProviderConfig{Provider: "openai_compatible", Model: "mistralai/ministral-14b-2512"}
	stale := measuredRecord(task, bound, profile, aicert.VerdictCertified, 0)
	stale.SiteThinking = map[string]string{"brief": "high"}
	tiers := map[ai.Tier]ai.ProviderConfig{}
	for _, tier := range ai.TaskLadder(task) {
		tiers[tier] = bound
	}
	preset := attributeOneTask(task, profile, tiers, stale)
	if preset.Tasks[0].Band != "" {
		t.Fatalf("the preset row graded a record measured at other thinking levels: %q", preset.Tasks[0].Band)
	}
	doc := aiCertDoc{Presets: []aiCertPreset{preset}, Sites: []aiCertSite{{
		Task: string(task), Key: string(task) + "/brief",
		Records: []aiCertRecord{{Binding: bindingRefOf(stale), State: aicert.StatusCurrent, Band: stale.Verdict, Runs: stale.Runs, Passed: stale.Passed}},
	}}}
	for _, fam := range buildAICertFamilies(doc, nil) {
		if fam.Ready != 0 || len(fam.Sites) != 0 {
			t.Errorf("family %s counted a record its preset calls unmeasured: ready %d over %d site(s)", fam.Name, fam.Ready, len(fam.Sites))
		}
	}
}
