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
	RecheckPending int `json:"recheck_pending"`
	// Models are the family's bindings as the by-model table has them.
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
}

// buildAICertFamilies folds the document's records by model family. A family is
// graded on its best model per feature, the model a buyer picking that family
// would run, so a weak model in the family cannot hide a strong one.
func buildAICertFamilies(doc aiCertDoc) []aiCertFamily {
	byName := map[string]*aiCertFamily{}
	for _, site := range doc.Sites {
		best := map[string]aiCertRecord{}
		count := map[string]int{}
		for _, rec := range eligibleFamilyRecords(site) {
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
				Runs: rec.Runs, Passed: rec.Passed, Records: count[name],
			})
		}
	}
	for _, b := range doc.Bindings {
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

// eligibleFamilyRecords drops what the product never serves: a local-only task
// skips hosted bindings, as the preset view does, so a hosted record cannot
// lift a family's answer for a feature a buyer would not get from it.
func eligibleFamilyRecords(site aiCertSite) []aiCertRecord {
	if !ai.LocalOnly(ai.Task(site.Task)) {
		return site.Records
	}
	var eligible []aiCertRecord
	for _, rec := range site.Records {
		if ai.ProviderIsLocal(rec.Binding.Provider) {
			eligible = append(eligible, rec)
		}
	}
	return eligible
}

func countAICertFamily(fam *aiCertFamily) {
	fam.SitesMeasured = len(fam.Sites)
	for _, s := range fam.Sites {
		switch s.Band {
		case aicert.VerdictCertified:
			fam.Ready++
		case aicert.VerdictSupportedDegraded:
			fam.UsableWithCare++
		default:
			fam.NotReliableYet++
		}
		if s.State == aicert.StatusStale {
			fam.RecheckPending++
		}
	}
	fam.Answer = familyAnswer(fam)
}

// familyAnswer grades a family, and both halves of the page state its rule: not enough tested
// when the family covers under half the features, yes when every measured one is
// ready, mostly when at least familyMostlyPercent are usable, not yet otherwise.
func familyAnswer(fam *aiCertFamily) string {
	switch {
	case fam.SitesMeasured*familyCoverageDivisor < fam.SitesShipped:
		return familyNotEnough
	case fam.Ready == fam.SitesMeasured:
		return familyYes
	case (fam.Ready+fam.UsableWithCare)*100 >= fam.SitesMeasured*familyMostlyPercent:
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
		return fmt.Sprintf("Tested on only %d of the %d features. Ask before relying on it.",
			fam.SitesMeasured, fam.SitesShipped)
	}
	words := fmt.Sprintf("Ready for %d of the %d features we tested", fam.Ready, fam.SitesMeasured)
	if fam.UsableWithCare > 0 {
		words += fmt.Sprintf("; %d more work if someone looks over the result", fam.UsableWithCare)
	}
	if fam.NotReliableYet > 0 {
		words += fmt.Sprintf("; %d not reliable yet", fam.NotReliableYet)
	}
	return words + "."
}

// familyModelNames names the models behind a family's answer, without the
// broker prefix or the environment, because a family name alone misleads: "GPT"
// here is an open-weight model, not the vendor's flagship.
func familyModelNames(fam aiCertFamily) string {
	var names []string
	seen := map[string]bool{}
	for _, m := range fam.Models {
		name := m.Binding.Model[strings.LastIndex(m.Binding.Model, "/")+1:]
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
	page.WriteString("**\"I want to run Margince on Gemini — can I trust it?\"** This is the answer for each\n")
	page.WriteString("family of AI models, across everything Margince does with AI. To start from a ready-made\n")
	page.WriteString("setup instead, go to [Can I use this preset?](#can-i-use-this-preset).\n\n")
	page.WriteString("| Family | Models we tested | Can I trust it? | In plain words |\n|---|---|---|---|\n")
	for _, fam := range families {
		fmt.Fprintf(page, "| %s | %s | %s | %s |\n", fam.Name, familyModelNames(fam), familyAnswerWords(fam.Answer), familyPlainWords(fam))
	}
	page.WriteString("\n")
	fmt.Fprintf(page, "**Yes** means ready for every feature we tested. **Mostly** means at least %d in every 100 "+
		"tested features are ready or work with a check. **Not yet** means fewer. **Not enough tested yet** "+
		"means we tested it on under half of the features, so we do not say. A family is judged on its best "+
		"model for each feature.\n\n", familyMostlyPercent)
}

// writeAICertFamilyDetail is the engineers' half: every model in each family and
// its best result on every feature it was measured on.
func writeAICertFamilyDetail(page *strings.Builder, families []aiCertFamily) {
	writeAICertFolded(page, "See the detail behind each family", func(page *strings.Builder) {
		for _, fam := range families {
			writeAICertFamilySection(page, fam)
		}
	})
}

func writeAICertFamilySection(page *strings.Builder, fam aiCertFamily) {
	fmt.Fprintf(page, "### %s\n\n", fam.Name)
	fmt.Fprintf(page, "%s — %s\n\n", familyAnswerWords(fam.Answer), familyPlainWords(fam))
	if fam.RecheckPending > 0 {
		fmt.Fprintf(page, "%d of these results were measured on an older version of the product and are re-check pending.\n\n",
			fam.RecheckPending)
	}
	page.WriteString("| Model | Where it ran | Features tested | ✅ | ⚠️ | ❌ | Tries right |\n|---|---|---:|---:|---:|---:|---|\n")
	for _, m := range fam.Models {
		fmt.Fprintf(page, "| `%s` | `%s` | %d | %d | %d | %d | %s |\n", m.Binding.Model, m.Binding.Env,
			m.Sites, m.Bands.Certified, m.Bands.SupportedDegraded, m.Bands.NotSupported, reliabilityCell(m.Reliability))
	}
	page.WriteString("\n<details>\n<summary>Best model for each feature</summary>\n\n")
	page.WriteString("| Feature | Best model in the family | Grade | Right in |\n|---|---|---|---|\n")
	for _, s := range fam.Sites {
		fmt.Fprintf(page, "| [`%s`](#%s) | `%s` | %s%s | %d of %d tries |\n", s.Key, aiCertSiteAnchor(s.Key),
			s.Best.label(), bandGrade(s.Band), staleMark(s.State), s.Passed, s.Runs)
	}
	page.WriteString("\n</details>\n\n")
}

func bandGrade(band string) string {
	switch band {
	case aicert.VerdictCertified:
		return aiCertReady
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
// same builder, so every record of every site must land in exactly one family
// and every measured binding in exactly one family's model table.
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
	for _, site := range doc.Sites {
		perFamily := map[string]int{}
		for _, rec := range eligibleFamilyRecords(site) {
			perFamily[aicert.ModelFamily(rec.Binding.Model)]++
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
		if seen[b.Binding.label()] != 1 {
			t.Errorf("binding %s is in %d families' model tables, want 1", b.Binding.label(), seen[b.Binding.label()])
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

func TestAHostedRecordDoesNotGradeAFamilyOnALocalOnlyTask(t *testing.T) {
	hosted := aiCertRecord{Binding: aiCertBindingRef{Provider: "gemini", Model: "gemini-3.5-flash"}, Band: aicert.VerdictCertified}
	local := aiCertRecord{Binding: aiCertBindingRef{Provider: "ollama", Model: "gemma4:12b"}, Band: aicert.VerdictCertified}
	localOnly := aiCertSite{Task: "capture_counterparty_verdict", Records: []aiCertRecord{hosted, local}}
	if got := eligibleFamilyRecords(localOnly); len(got) != 1 || got[0].Binding.Provider != "ollama" {
		t.Errorf("a local-only task kept %+v, want only the local record", got)
	}
	open := aiCertSite{Task: "site_triage", Records: []aiCertRecord{hosted, local}}
	if got := eligibleFamilyRecords(open); len(got) != 2 {
		t.Errorf("a task with no locality rule kept %d records, want both", len(got))
	}
}
