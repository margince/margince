// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A project's history is readable like every other record's: the phase move
// the deals store writes (action advance_phase, with the phase before/after
// images) comes back from both history views — the whole-mutation record
// history and the per-field diff — naming who moved it and when.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const phaseMoveReason = "Offer approved after the site visit."

func TestProjectHistoryListsThePhaseTransitionAndWhoMadeIt(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "BAER Pharma", nil)
	p := seedProject(e.Admin(), t, e, "ERP replacement", company, &e.Rep1)

	mover := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"project": {Read: true, Update: true}},
		RowScope: principal.RowScopeOwn,
	})
	if _, err := e.Projects.AdvanceProjectPhase(mover, p.ID, projects.AdvanceProjectPhaseInput{
		ToPhase: projects.PhasePursuing, Reason: new(phaseMoveReason),
	}); err != nil {
		t.Fatalf("advance project: %v", err)
	}

	// The record history carries the creation row and the move. Removing
	// "project" from privacy's fieldHistoryEntityTypes makes this read
	// answer not-found.
	record, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "project", EntityID: p.ID.UUID,
	})
	if err != nil {
		t.Fatalf("record history of a project: %v", err)
	}
	// The project's OWN lines are the creation and the move. The third is the
	// company its creation linked: a record's history includes the links it holds,
	// so a count over the whole page would absorb a missing line of the project's
	// own the moment a link row appeared beside it.
	var own []privacy.RecordHistoryEntry
	var links int
	for _, entry := range record.Entries {
		if entry.Edge != nil {
			links++
			continue
		}
		own = append(own, entry)
	}
	if len(own) != 2 {
		t.Fatalf("the project's own history has %d entries, want 2 (create, advance_phase): %+v",
			len(own), record.Entries)
	}
	if links != 1 {
		t.Fatalf("the project's history carries %d link lines, want the one company its "+
			"creation linked: %+v", links, record.Entries)
	}
	var move *privacy.RecordHistoryEntry
	for i := range own {
		if own[i].Action == "advance_phase" {
			move = &own[i]
		}
	}
	if move == nil {
		t.Fatalf("no advance_phase line in %+v", record.Entries)
	}
	if move.ActorID != "human:"+e.Rep1.String() {
		t.Errorf("the move is attributed to %q, want the rep who made it (human:%s)", move.ActorID, e.Rep1)
	}
	if move.Before["phase"] != projects.PhaseInitiative || move.After["phase"] != projects.PhasePursuing {
		t.Errorf("phase images = %v → %v, want initiative → pursuing", move.Before["phase"], move.After["phase"])
	}
	if move.OccurredAt.IsZero() {
		t.Error("the move carries no occurred_at")
	}
	if move.Reason == nil || *move.Reason != phaseMoveReason {
		t.Errorf("the move's reason = %v, want the one given with it (%q)", move.Reason, phaseMoveReason)
	}

	// The per-field diff: advance_phase is a projected verb, so the phase
	// change reads as one field entry. Removing "advance_phase" from
	// privacy's fieldHistoryProjectedActions leaves this page empty.
	fields, err := privacy.ListFieldHistory(e.Admin(), e.DB(), privacy.FieldHistoryFilter{
		EntityType: "project", EntityID: p.ID.UUID, Field: new("phase"),
	})
	if err != nil {
		t.Fatalf("field history of a project: %v", err)
	}
	if len(fields.Entries) != 1 {
		t.Fatalf("phase field history has %d entries, want exactly the one move: %+v", len(fields.Entries), fields.Entries)
	}
	if got := fields.Entries[0]; got.OldValue == nil || got.NewValue == nil ||
		*got.OldValue != projects.PhaseInitiative || *got.NewValue != projects.PhasePursuing {
		t.Errorf("phase diff = %v → %v, want initiative → pursuing", got.OldValue, got.NewValue)
	}
}

// The reason rides with the phase it explains: a role that does not read the
// phase reads no reason for moving it.
func TestAMaskedPhaseHidesTheReasonForTheMove(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Elbe Kälte", nil)
	p := seedProject(e.Admin(), t, e, "Cold-room rollout", company, &e.Rep1)
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), p.ID, projects.AdvanceProjectPhaseInput{
		ToPhase: projects.PhasePursuing, Reason: new(phaseMoveReason),
	}); err != nil {
		t.Fatalf("advance project: %v", err)
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"project": {Read: true}},
		RowScope: principal.RowScopeTeam,
		FieldMasks: []principal.FieldMask{
			{Object: "project", Field: "phase", Condition: principal.MaskAlways},
		},
	})
	page, err := privacy.ListRecordHistory(reader, e.DB(), privacy.RecordHistoryFilter{
		EntityType: "project", EntityID: p.ID.UUID,
	})
	if err != nil {
		t.Fatalf("record history of a project: %v", err)
	}
	moves := 0
	for _, entry := range page.Entries {
		if entry.Action != "advance_phase" {
			continue
		}
		moves++
		if entry.Reason != nil {
			t.Errorf("a reader without the phase read the reason for moving it: %q", *entry.Reason)
		}
	}
	if moves == 0 {
		t.Fatal("the reader saw no phase move, so nothing proves the mask")
	}
}
