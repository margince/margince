// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A release correction re-files old mail by writing one activity_relink audit
// row per message, under the actor system:datatask:<task id> and with the
// step's run id as batch_id. On a copy of a live installation one run wrote
// 366 of them in a day, and the page read as hundreds of things done for the
// reader, "by Margince", with no reason.

const relinkTask = "system:datatask:core/1792680000_captured_mail_filed_on_its_deal"

func relinkRow(run ids.UUID, at time.Time) entry {
	subject := "Re: rollout plan"
	return entry{
		ID: ids.NewV7(), OccurredAt: at, Action: "activity_relink", EntityType: "activity",
		EntityID: ids.NewV7(), ActorType: "system", ActorID: relinkTask, Label: &subject,
		BatchID: &run,
	}
}

func TestOneCorrectionRunIsOneMaintenanceLineBesideARealOutcome(t *testing.T) {
	now := time.Now()
	run := ids.NewV7()
	var entries []entry
	for i := range 3000 {
		entries = append(entries, relinkRow(run, now.Add(-time.Duration(i)*time.Second)))
	}
	// A real piece of work for the reader, older than every relink, so a page
	// that showed relinks first and cut at its limit would lose it.
	filing := auditRow("link-reconcile", "", `{"cohort_linked": 0, "cohort_promoted": 1}`, "", now.Add(-2*time.Hour))
	entries = append(entries, filing)

	lines, _ := linesOf(unmasked(t), entries, nil, 50)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want one correction line and one mail filing", len(lines))
	}
	var correction, outcome int
	for i, l := range lines {
		if l.Maintenance != nil && *l.Maintenance {
			correction = i
		} else {
			outcome = i
		}
	}
	c := lines[correction]
	if c.Count == nil || *c.Count != 3000 {
		t.Fatalf("the correction line counts %v records, want 3000", c.Count)
	}
	if c.Actor.Label == nil || c.Actor.Label.Key != byDataCorrection {
		t.Fatalf("the correction is labelled %v, want the data-correction actor", c.Actor.Label)
	}
	if c.Reason == nil || c.Reason.Key != whyDataCorrection {
		t.Fatalf("the correction carries reason %v, want that it corrected older records", c.Reason)
	}
	o := lines[outcome]
	if o.Maintenance != nil && *o.Maintenance {
		t.Fatal("the mail filing was marked as maintenance")
	}
	if o.Summary.Key != "magic.action.mail_filed" || (o.Count != nil && *o.Count != 1) {
		t.Fatalf("the real outcome reads %q x%v, want the mail filing once", o.Summary.Key, o.Count)
	}
}

func TestTwoRunsOfOneCorrectionAreTwoLines(t *testing.T) {
	now := time.Now()
	first, second := ids.NewV7(), ids.NewV7()
	entries := []entry{
		relinkRow(first, now), relinkRow(first, now.Add(-time.Second)),
		relinkRow(second, now.Add(-time.Minute)),
	}
	lines, _ := linesOf(unmasked(t), entries, nil, 50)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want one per correction run", len(lines))
	}
	counts := map[int]bool{}
	for _, l := range lines {
		n := 1
		if l.Count != nil {
			n = *l.Count
		}
		counts[n] = true
	}
	if !counts[2] || !counts[1] {
		t.Fatalf("run counts %v, want 2 and 1", counts)
	}
}

// A row a data task wrote before batches were stamped still folds into one
// bounded line, still reads as a correction, and claims no run of its own.
func TestALegacyCorrectionRowWithoutARunStillFoldsAsMaintenance(t *testing.T) {
	now := time.Now()
	a, b := relinkRow(ids.NewV7(), now), relinkRow(ids.NewV7(), now.Add(-time.Second))
	a.BatchID, b.BatchID = nil, nil
	lines, _ := linesOf(unmasked(t), []entry{a, b}, nil, 50)
	if len(lines) != 1 || lines[0].Count == nil || *lines[0].Count != 2 {
		t.Fatalf("got %+v, want one line of 2", lines)
	}
	if lines[0].Maintenance == nil || !*lines[0].Maintenance {
		t.Fatal("a legacy correction row lost its maintenance mark")
	}
}

// The detail a correction line opens to is that run's records and no other's:
// the count and the list are one answer.
func TestACorrectionLineOpensToItsOwnRunOnly(t *testing.T) {
	now := time.Now()
	first, second := ids.NewV7(), ids.NewV7()
	mine := []entry{relinkRow(first, now), relinkRow(first, now.Add(-time.Second))}
	other := relinkRow(second, now.Add(-time.Minute))
	members, ok := lineMembers(append(append([]entry{}, mine...), other), mine[0].ID)
	if !ok || len(members) != 2 {
		t.Fatalf("members = %d (ok %t), want the run's 2 records", len(members), ok)
	}
	for _, m := range members {
		if m.EntityID == other.EntityID {
			t.Fatal("another run's record leaked into this line's detail")
		}
	}
}

// A job that is not a data task keeps its own label and gets no maintenance
// mark, even when its rows carry a batch.
func TestABatchAloneDoesNotMakeALineMaintenance(t *testing.T) {
	run := ids.NewV7()
	filing := auditRow("link-reconcile", "", `{"cohort_linked": 0, "cohort_promoted": 1}`, "", time.Now())
	filing.BatchID = &run
	line, _, ok := lineOf(unmasked(t), filing)
	if !ok {
		t.Fatal("the filing was not shown")
	}
	if line.Maintenance != nil || line.Actor.Label == nil || line.Actor.Label.Key != "magic.by.mail_filing" {
		t.Fatalf("a batched mail filing reads as %v / %v", line.Maintenance, line.Actor.Label)
	}
}
