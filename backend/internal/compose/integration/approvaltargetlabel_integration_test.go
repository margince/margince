// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a staged proposal remembers its target being called.
//
// Roughly half the stageable kinds carry no typed payload — the raw args a tool
// was called with, an automation action's, a canonicalized HTTP body — so the
// card has nothing but the summary to work from, and the summaries those paths
// compose name the record by uuid. The label is what lets the card say which
// record without the reader opening the payload.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stagedLabel stages one proposal and reads back what it recorded the target as
// being called, through the same wire the inbox serves.
func stagedLabel(t *testing.T, e *Env, svc *approvals.Service, kind, targetType string, target ids.UUID) *string {
	t.Helper()
	change, hash, err := diffhash.Canonical(json.RawMessage(`{"note": "staged"}`))
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Stage(e.AgentCtx(), approvals.StageInput{
		Kind: kind, ProposedChange: change, DiffHash: hash,
		TargetType: targetType, TargetID: target, Summary: "a summary naming nothing",
	})
	if err != nil {
		t.Fatalf("staging: %v", err)
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	rows, _, err := svc.ListWire(rep, approvals.ListInput{})
	if err != nil {
		t.Fatalf("reading the inbox: %v", err)
	}
	for _, row := range rows {
		if ids.UUID(row.Id) == id.UUID {
			return row.TargetLabel
		}
	}
	t.Fatal("the staged proposal is absent from the inbox, so this proved nothing")
	return nil
}

// A PROPOSAL REMEMBERS WHAT ITS TARGET IS CALLED.
//
// Without it the reader of an untyped kind gets a verb and a uuid, and decides
// on the verb alone.
func TestAStagedProposalRemembersWhatItsTargetIsCalled(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Weber GmbH — Phase 2", pipeline, open, &e.Rep1)

	label := stagedLabel(t, e, approvals.NewService(e.DB()), "advance_deal", "deal", deal)
	if label == nil || *label != "Weber GmbH — Phase 2" {
		t.Errorf("target_label = %v, want the deal's own name — a card with only the uuid asks "+
			"somebody to approve a change to a record it will not name", label)
	}
}

// AND IT IS THE NAME AT STAGING TIME, not at reading time.
//
// The record may be renamed before anyone opens the inbox, and a caption
// resolved then would put a word in front of the approver that the proposal was
// never about.
func TestTheLabelIsTheNameTheProposalWasStagedAgainst(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Weber GmbH — Phase 2", pipeline, open, &e.Rep1)

	svc := approvals.NewService(e.DB())
	change, hash, err := diffhash.Canonical(json.RawMessage(`{"note": "staged"}`))
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Stage(e.AgentCtx(), approvals.StageInput{
		Kind: "advance_deal", ProposedChange: change, DiffHash: hash,
		TargetType: "deal", TargetID: deal, Summary: "a summary naming nothing",
	})
	if err != nil {
		t.Fatalf("staging: %v", err)
	}
	if _, err := owner.Exec(e.AgentCtx(),
		`UPDATE deal SET name = 'Renamed after staging' WHERE id = $1`, deal); err != nil {
		t.Fatalf("renaming the deal: %v", err)
	}

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	rows, _, err := svc.ListWire(rep, approvals.ListInput{})
	if err != nil {
		t.Fatalf("reading the inbox: %v", err)
	}
	for _, row := range rows {
		if ids.UUID(row.Id) != id.UUID {
			continue
		}
		if row.TargetLabel == nil || *row.TargetLabel != "Weber GmbH — Phase 2" {
			t.Errorf("target_label = %v after a rename, want the name the proposal was staged "+
				"against — the approver is being shown a record they were not asked about", row.TargetLabel)
		}
		return
	}
	t.Fatal("the staged proposal is absent from the inbox")
}

// A TARGET TYPE WITH NO NAME RECORDS NOTHING, rather than a blank or an
// "unknown": absence is what tells the card to say nothing at all.
func TestATargetWithNoNameRecordsNoLabel(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	activity := ids.NewV7()
	if _, err := owner.Exec(e.AgentCtx(), `
		INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'note', NULL, 'a note', now(), 'seed', 'test')`, activity); err != nil {
		t.Fatalf("seeding an activity: %v", err)
	}

	svc := approvals.NewService(e.DB())
	change, hash, err := diffhash.Canonical(json.RawMessage(`{"note": "staged"}`))
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Stage(e.AgentCtx(), approvals.StageInput{
		Kind: "relink_activity", ProposedChange: change, DiffHash: hash,
		TargetType: "activity", TargetID: activity, Summary: "a summary naming nothing",
	})
	if err != nil {
		t.Fatalf("staging: %v", err)
	}
	// Read straight off the row: what this case is about is what was STORED,
	// and an activity's kind reaches a narrower inbox than the deal cases above.
	var label *string
	if err := owner.QueryRow(e.AgentCtx(),
		`SELECT target_label FROM approval WHERE id = $1`, id.UUID).Scan(&label); err != nil {
		t.Fatalf("reading the staged row: %v", err)
	}
	if label != nil {
		t.Errorf("target_label = %q for an activity, want none: a timeline entry has no name a "+
			"person calls it by, and inventing one would caption the card with a word its "+
			"reader never uses", *label)
	}
}
