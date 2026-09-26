// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func auditRow(actor, action, before, after, evidence string, at time.Time) entry {
	name := "Anna Keller"
	e := entry{
		ID: ids.NewV7(), OccurredAt: at, Action: action, EntityType: "contact",
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
	line, _, ok := lineOf(auditRow("link-reconcile", "update", "", `{"cohort_linked": 0, "cohort_promoted": 1}`, "", time.Now()))
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
	line, _, ok := lineOf(auditRow("agent:deepread", "update",
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

// An update that moved nothing a reader cares about is not a line.
func TestAnUpdateThatSaysNothingIsNotShown(t *testing.T) {
	for _, e := range []entry{
		auditRow("system", "update", "", "", "", time.Now()),
		auditRow("agent:knowledge-ingest", "update", `{"chunk_count": 0}`, `{"chunk_count": 25}`, "", time.Now()),
		auditRow("system", "update", `{"stage": "a"}`, `{"stage": "a"}`, "", time.Now()),
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
		entries = append(entries, auditRow("link-reconcile", "update", "", `{"cohort_linked": 1, "cohort_promoted": 0}`, "",
			now.Add(-time.Duration(i)*time.Second)))
	}
	entries = append(entries, auditRow("system", "update", "", "", "", now))
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
	first := auditRow("link-reconcile", "update", "", `{"cohort_linked": 1, "cohort_promoted": 0}`, "", now)
	again := first
	again.ID = ids.NewV7()
	again.OccurredAt = now.Add(-time.Minute)
	other := auditRow("link-reconcile", "update", "", `{"cohort_linked": 1, "cohort_promoted": 0}`, "", now.Add(-2*time.Minute))
	lines, _ := linesOf([]entry{first, again, other}, 100)
	if len(lines) != 1 || lines[0].Count == nil || *lines[0].Count != 2 {
		t.Fatalf("got %d lines with count %v, want one line counting the two distinct records", len(lines), lines[0].Count)
	}
}
