// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// auditRow is one machine update of a contact named Anna Keller.
func auditRow(actor, before, after, evidence string, at time.Time) entry {
	name := "Anna Keller"
	e := entry{
		ID: ids.NewV7(), OccurredAt: at, Action: actionUpdate, EntityType: "contact",
		EntityID: ids.NewV7(), ActorType: "system", ActorID: actor, Label: &name,
	}
	if before != "" {
		e.Before = []byte(before)
	}
	if after != "" {
		e.After = []byte(after)
	}
	if evidence != "" {
		e.Evidence = []byte(evidence)
	}
	return e
}

// Mail filed under a contact reads as that, says why, and names the job.
func TestAMailFilingSaysWhatItDidAndWhy(t *testing.T) {
	line, _, ok := lineOf(auditRow("link-reconcile", "", `{"cohort_linked": 0, "cohort_promoted": 1}`, "", time.Now()))
	if !ok {
		t.Fatal("a mail filing was not shown")
	}
	if line.Summary.Key != "magic.action.mail_filed" || line.Reason == nil || line.Reason.Key != "magic.why.mail_filed" {
		t.Fatalf("summary %q reason %v, want the mail-filing sentence and its reason", line.Summary.Key, line.Reason)
	}
	if line.Actor.Label == nil || line.Actor.Label.Key != "magic.by.mail_filing" {
		t.Fatalf("actor label %v, want the mail-filing job", line.Actor.Label)
	}
	if line.Entity == nil || line.Entity.Label == nil || *line.Entity.Label != "Anna Keller" {
		t.Fatal("the line does not name the contact")
	}
}

// An enrichment names the fields it changed and the page it read them on.
func TestAFieldChangeNamesTheFieldsAndTheSource(t *testing.T) {
	line, _, ok := lineOf(auditRow("agent:deepread",
		`{"role": null, "title": null}`, `{"role": "Counsel", "title": "Counsel"}`,
		`{"source": "site_read", "source_ref": "site_read:https://www.studiolegal.de/de/team"}`, time.Now()))
	if !ok {
		t.Fatal("a field change was not shown")
	}
	if line.Summary.Key != "magic.action.fields_changed" || (*line.Summary.Values)["fields"] != "role, title" {
		t.Fatalf("summary %q values %v, want the changed fields", line.Summary.Key, line.Summary.Values)
	}
	if line.Reason == nil || line.Reason.Key != "magic.why.site_read" || (*line.Reason.Values)["site"] != "studiolegal.de" {
		t.Fatalf("reason %v, want the site it was read on", line.Reason)
	}
}

// The website reader records the page it read as source_url, which is the
// shape every live row carries.
func TestASiteReadNamesTheSiteItRecordedAsSourceURL(t *testing.T) {
	line, _, ok := lineOf(auditRow("agent:deepread",
		`{"industry": null}`, `{"industry": "Legal services"}`,
		`{"source": "site_read", "source_url": "https://www.studiolegal.de/de/about"}`, time.Now()))
	if !ok {
		t.Fatal("a site read was not shown")
	}
	if line.Reason == nil || line.Reason.Key != "magic.why.site_read" || (*line.Reason.Values)["site"] != "studiolegal.de" {
		t.Fatalf("reason %v, want the site it was read on", line.Reason)
	}
}

// A logo row records the IMAGE's address, often on a CDN; naming that host as
// the site would be wrong, so the reason says the company's website instead.
func TestALogoReadDoesNotNameTheImageHostAsTheSite(t *testing.T) {
	logo := "https://cdn.prod.website-files.com/66ded54b/webclip.png"
	line, _, ok := lineOf(auditRow("agent:deepread",
		`{"logo": null}`, `{"logo": "`+logo+`"}`,
		`{"source": "site_read", "source_url": "`+logo+`"}`, time.Now()))
	if !ok {
		t.Fatal("a logo read was not shown")
	}
	if line.Reason == nil || line.Reason.Key != "magic.why.site_read_unnamed" || line.Reason.Values != nil {
		t.Fatalf("reason %v, want the unnamed website reason", line.Reason)
	}
}

// One website reader run over many companies is one line, and that line does
// not name one company's site as the source for all of them.
func TestASiteReadOverManyRecordsIsOneLineWithoutOneSite(t *testing.T) {
	now := time.Now()
	var entries []entry
	for i, site := range []string{"https://a.example/about", "https://b.example/about", "https://c.example/logo.png"} {
		after := `{"industry": "Software"}`
		if i == 2 {
			after = `{"industry": "Software", "logo": "` + site + `"}`
		}
		entries = append(entries, auditRow("agent:deepread", `{"industry": null}`, after,
			`{"source": "site_read", "source_url": "`+site+`"}`, now.Add(-time.Duration(i)*time.Second)))
	}
	lines, _ := linesOf(entries, 100)
	var industry []int
	for i, l := range lines {
		if (*l.Summary.Values)["fields"] == "industry" {
			industry = append(industry, i)
		}
	}
	if len(industry) != 1 {
		t.Fatalf("got %d industry lines, want the two page reads folded into one", len(industry))
	}
	l := lines[industry[0]]
	if l.Count == nil || *l.Count != 2 || l.Reason == nil || l.Reason.Key != "magic.why.site_read_each" {
		t.Fatalf("count %v reason %v, want 2 records read on each company's own website", l.Count, l.Reason)
	}
}

// One record read from two sites names neither, rather than the newer alone.
func TestOneRecordReadFromTwoSitesNamesNeither(t *testing.T) {
	now := time.Now()
	first := auditRow("agent:deepread", `{"industry": null}`, `{"industry": "Software"}`,
		`{"source": "site_read", "source_url": "https://a.example/about"}`, now)
	second := first
	second.ID = ids.NewV7()
	second.OccurredAt = now.Add(-time.Minute)
	second.Evidence = []byte(`{"source": "site_read", "source_url": "https://b.example/about"}`)
	lines, _ := linesOf([]entry{first, second}, 100)
	if len(lines) != 1 || lines[0].Reason == nil || lines[0].Reason.Key != "magic.why.site_read_unnamed" {
		t.Fatalf("got %d lines, reason %v; want one line naming no single site", len(lines), lines[0].Reason)
	}
}

// A website field whose value IS the page read still names that site: only
// the logo's address is an image rather than a page.
func TestAWebsiteReadThatWritesTheWebsiteNamesTheSite(t *testing.T) {
	site := "https://www.studiolegal.de"
	line, _, ok := lineOf(auditRow("agent:deepread", `{"website": null}`, `{"website": "`+site+`"}`,
		`{"source": "site_read", "source_url": "`+site+`"}`, time.Now()))
	if !ok || line.Reason == nil || line.Reason.Key != "magic.why.site_read" || (*line.Reason.Values)["site"] != "studiolegal.de" {
		t.Fatalf("reason %v, want the site it was read on", line.Reason)
	}
}

// The mail reader's reply sorting is bookkeeping, not a change to report.
func TestTheMailReadersReplySortingIsNotShown(t *testing.T) {
	e := auditRow("system:owed_verdict", `{"owed_verdict": null, "owed_verdict_ruleset": null}`,
		`{"owed_verdict": "informs_us", "owed_verdict_ruleset": "prompts-82e2"}`, "", time.Now())
	e.EntityType = "activity"
	if _, _, ok := lineOf(e); ok {
		t.Fatal(`the mail reader's reply sorting was shown as "Changed owed verdict"`)
	}
}

// An update that moved nothing a reader cares about is not a line.
func TestAnUpdateThatSaysNothingIsNotShown(t *testing.T) {
	for _, e := range []entry{
		auditRow("system", "", "", "", time.Now()),
		auditRow("agent:knowledge-ingest", `{"chunk_count": 0}`, `{"chunk_count": 25}`, "", time.Now()),
		auditRow("system", `{"stage": "a"}`, `{"stage": "a"}`, "", time.Now()),
	} {
		if _, _, ok := lineOf(e); ok {
			t.Errorf("%s %s after=%s was shown; it changed nothing a reader can use", e.ActorID, e.Action, e.After)
		}
	}
}

// One job doing one thing to many records is one line with a count, counted
// over every row read rather than over the page's line limit.
func TestOneJobOnManyRecordsIsOneLineWithACount(t *testing.T) {
	now := time.Now()
	var entries []entry
	for i := range 250 {
		entries = append(entries, auditRow("link-reconcile", "", `{"cohort_linked": 1, "cohort_promoted": 0}`, "",
			now.Add(-time.Duration(i)*time.Second)))
	}
	entries = append(entries, auditRow("system", "", "", "", now))
	lines, housekeeping := linesOf(entries, 100)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want the job folded into one", len(lines))
	}
	if lines[0].Count == nil || *lines[0].Count != 250 {
		t.Fatalf("count %v, want 250 — every row of the job, not the page limit", lines[0].Count)
	}
	if housekeeping != 1 {
		t.Fatalf("housekeeping %d, want the one row that said nothing counted", housekeeping)
	}
}

// Two passes of one job over the same record count that record once.
func TestAGroupCountsRecordsNotAuditRows(t *testing.T) {
	now := time.Now()
	first := auditRow("link-reconcile", "", `{"cohort_linked": 1, "cohort_promoted": 0}`, "", now)
	again := first
	again.ID = ids.NewV7()
	again.OccurredAt = now.Add(-time.Minute)
	other := auditRow("link-reconcile", "", `{"cohort_linked": 1, "cohort_promoted": 0}`, "", now.Add(-2*time.Minute))
	lines, _ := linesOf([]entry{first, again, other}, 100)
	if len(lines) != 1 || lines[0].Count == nil || *lines[0].Count != 2 {
		t.Fatalf("got %d lines with count %v, want one line counting the two distinct records", len(lines), lines[0].Count)
	}
}
