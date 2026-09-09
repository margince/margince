// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Evidence is frozen so it can outlive the deal or project it names — that is
// why the qualifying record's NAME is copied into the row rather than joined
// for. The id beside it is ON DELETE SET NULL, so once the record is gone the
// name is all that tells two rows on one activity apart, and two same-named
// records collapse onto one uniqueness key.
//
// What breaks is not the evidence but the DELETE: the second one is refused by
// a unique violation on a row nobody touched. These tests drive the real
// stamping writers and then delete through the FK, which is the only way a deal
// or a project is hard-deleted — the product archives rather than deletes, so
// there is no handler to call and the constraint itself is the subject.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Two projects with one name, both deleted. The second delete used to be
// refused, leaving a project a controller cannot remove for a reason that names
// no project.
func TestDeletingTwoSameNamedProjectsLeavesBothEvidenceRowsStanding(t *testing.T) {
	e := Setup(t)
	f := seedProjectStampFixture(t, e)

	// The SAME name on purpose. Project names are not unique — only `key` is,
	// and only among unarchived rows — so two live projects called this is an
	// ordinary state rather than a contrived one.
	second := ids.NewV7()
	e.WsExec(t, `INSERT INTO project (id, name, organization_id, phase, source, captured_by)
		SELECT $1, name, organization_id, 'delivering', 'manual', 'human:x'
		  FROM project WHERE id = $2`, second, f.project)

	for _, target := range []ids.UUID{f.project, second} {
		if _, err := e.Activities.RelinkActivity(e.Admin(), ids.ActivityID{UUID: f.email},
			activities.RelinkActivityInput{
				EntityType: "project", EntityID: target, ReplaceExistingOfType: true,
			}); err != nil {
			t.Fatalf("filing the email under project %v: %v", target, err)
		}
	}

	for _, target := range []ids.UUID{f.project, second} {
		if err := e.WsExecErr(t, `DELETE FROM project WHERE id = $1`, target); err != nil {
			t.Fatalf("deleting project %v: %v — evidence left behind must not make a project undeletable", target, err)
		}
	}

	// Both rows still there, both still naming what qualified them, both with
	// the reference cleared — the last of those is what says the deletes really
	// went through the FK rather than the test asserting its own setup.
	if got := e.WsCount(t, `SELECT count(*) FROM activity_retention_evidence
		 WHERE activity_id = $1 AND basis = 'project_linked'
		   AND project_name = 'ERP rollout' AND project_id IS NULL`, f.email); got != 2 {
		t.Fatalf("surviving project_linked rows = %d, want 2 — each project's evidence outlives it, or the obligation it recorded is lost", got)
	}
}

// The deal arm of the same shape, held separately because it is a different
// writer reached through a different event: a project qualifies correspondence
// when the LINK is written, a deal when it CONCLUDES.
func TestDeletingTwoSameNamedDealsLeavesBothEvidenceRowsStanding(t *testing.T) {
	e := Setup(t)
	f := seedProjectStampFixture(t, e)

	pipeline, openStage, wonStage := ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Same-named deals fixture', false, 93)`, pipeline)
	e.WsExec(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, openStage, pipeline)
	e.WsExec(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Closed Won', 1, 'won', 100)`, wonStage, pipeline)

	// Won through the real transition, because the deal stamp fires inside it.
	// A hand-inserted `status = 'won'` would leave the correspondence unstamped
	// and this test would pass against a schema holding no evidence at all.
	var seeded []ids.UUID
	for range 2 {
		deal := e.SeedDeal(t, "Renewal 2027",
			ids.PipelineID{UUID: pipeline}, ids.StageID{UUID: openStage}, nil)
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id)
			VALUES ($1, 'deal', $2)`, f.email, deal)
		if _, err := e.Deals.AdvanceDeal(e.Admin(), ids.DealID{UUID: deal},
			wonInput(ids.StageID{UUID: wonStage})); err != nil {
			t.Fatalf("winning deal %v: %v", deal, err)
		}
		seeded = append(seeded, deal)
	}

	for _, deal := range seeded {
		if err := e.WsExecErr(t, `DELETE FROM deal WHERE id = $1`, deal); err != nil {
			t.Fatalf("deleting deal %v: %v — evidence left behind must not make a deal undeletable", deal, err)
		}
	}

	if got := e.WsCount(t, `SELECT count(*) FROM activity_retention_evidence
		 WHERE activity_id = $1 AND basis = 'deal_won'
		   AND deal_name = 'Renewal 2027' AND deal_id IS NULL`, f.email); got != 2 {
		t.Fatalf("surviving deal_won rows = %d, want 2 — each deal's evidence outlives it, or the obligation it recorded is lost", got)
	}
}

// The premise the narrowed index rests on. Uniqueness stops covering a row once
// its reference is cleared, which is only safe because no INSERT can arrive
// with one already cleared: every writer joins a live deal or project to read
// the name it freezes. This guard is what refuses a writer that stops doing so,
// and without it the index would admit the duplicate it exists to prevent.
func TestDerivedEvidenceMustNameTheRecordItIsDerivedFrom(t *testing.T) {
	e := Setup(t)
	f := seedProjectStampFixture(t, e)

	err := e.WsExecErr(t, `INSERT INTO activity_retention_evidence
		  (activity_id, basis, qualified_at, project_name)
		VALUES ($1, 'project_linked', now(), 'A project named by name alone')`, f.email)
	if err == nil {
		t.Fatal("a project_linked row naming no project id was accepted; nothing deduplicates such a row, because the uniqueness index no longer covers it")
	}
	if !strings.Contains(err.Error(), "does not name") {
		t.Errorf("refused with %v, want the derived-evidence guard — another constraint firing first would leave this one unproven", err)
	}

	// The pin is the one basis that legitimately names no record: it is a
	// controller's decision about an activity, and the attribution is what
	// substantiates it. The guard has to let it through.
	if err := e.WsExecErr(t, `INSERT INTO activity_retention_evidence
		  (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, f.email); err != nil {
		t.Fatalf("the guard refused a controller pin: %v — a pin names no deal or project by design", err)
	}
}

// The other half of the premise: a reference is cleared by the FK or not at
// all.
//
// The narrowing only holds while a cleared reference means the record is GONE.
// The freeze trigger used to admit any non-null-to-null update, so a statement
// clearing an id by hand would take the row out of the index quietly and the
// next stamp for the same record would insert beside it instead of collapsing
// — the narrowing would have opened a hole rather than closed one.
//
// ON DELETE SET NULL fires once the parent is already gone, which is what lets
// one condition tell the two apart.
func TestAnEvidenceReferenceIsClearedByItsForeignKeyOrNotAtAll(t *testing.T) {
	e := Setup(t)
	f := seedProjectStampFixture(t, e)

	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.ActivityID{UUID: f.email},
		activities.RelinkActivityInput{EntityType: "project", EntityID: f.project}); err != nil {
		t.Fatalf("filing the email under its project: %v", err)
	}

	err := e.WsExecErr(t, `UPDATE activity_retention_evidence SET project_id = NULL
		 WHERE activity_id = $1 AND basis = 'project_linked'`, f.email)
	if err == nil {
		t.Fatal("an evidence row's project reference was cleared while the project still exists; that row has left the uniqueness index and the next stamp for the same project will duplicate it rather than collapse")
	}
	if !strings.Contains(err.Error(), "frozen") {
		t.Errorf("refused with %v, want the freeze trigger — another constraint firing first would leave this one unproven", err)
	}

	// The same update, once the project is gone, is the FK's own and must go
	// through. Asserting only the refusal above would pass against a trigger
	// that refused every clearing, which is a database no project can be
	// deleted from.
	if err := e.WsExecErr(t, `DELETE FROM project WHERE id = $1`, f.project); err != nil {
		t.Fatalf("deleting the project: %v — the FK's own clearing must still be admitted", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_retention_evidence
		 WHERE activity_id = $1 AND basis = 'project_linked' AND project_id IS NULL`, f.email); got != 1 {
		t.Fatalf("cleared project_linked rows = %d, want 1 — the FK could not clear the reference it owns", got)
	}
}
