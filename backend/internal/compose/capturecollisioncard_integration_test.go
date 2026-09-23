// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the capture-collision card SHOWS the human deciding it.
//
// Accepting fills the lead's empty fields and leaves every occupied one alone
// (contacts.FillEmptyLeadFieldsTx). A card carrying only the captured values
// therefore asks for a decision whose outcome the reader cannot see: three of
// the four values on screen may be dropped on accept, silently, and nothing
// distinguishes them from the one that lands.
//
// So the payload carries what the incumbent already holds for each foldable
// field. Through the real sink, because the pairing is only true if the values
// come off the row the collision actually found.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// capturedLeadWithFields is capturedLead for a record that carries more than a
// name: the collision card is about the fields, so the fields have to differ.
func (e *mergeEnv) capturedLeadWithFields(t *testing.T, source, sourceID string, fields capture.LeadFields) ids.UUID {
	t.Helper()
	ref, err := e.sink.Upsert(connectorCtx(e.Env), connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: source, SourceID: sourceID},
		Fields:     fields,
		Source:     source + ":" + sourceID, CapturedBy: "connector:test",
	})
	if err != nil {
		t.Fatalf("capturing %s/%s: %v", source, sourceID, err)
	}
	return ref.ID
}

// stagedChange is the payload of the merge proposal standing against one lead.
func (e *mergeEnv) stagedChange(t *testing.T, leadID ids.UUID) map[string]any {
	t.Helper()
	var raw json.RawMessage
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT proposed_change FROM approval WHERE kind = 'merge_records'
			   AND target_entity_id = $1 AND status = 'pending'`, leadID).Scan(&raw)
	}); err != nil {
		t.Fatalf("no staged merge to read: %v", err)
	}
	var change map[string]any
	if err := json.Unmarshal(raw, &change); err != nil {
		t.Fatalf("decoding the staged payload: %v", err)
	}
	return change
}

// The card draws both halves: what the message said, and what the lead holds.
//
// The incumbent is built with one field filled and one left empty on purpose.
// A payload that carried only the occupied values would pass a test that
// checked the conflicting field alone — and the empty one is the half that
// tells the reader the accept will do something.
func TestACaptureCollisionCardCarriesWhatTheLeadAlreadyHolds(t *testing.T) {
	e := setupMerge(t)
	incumbent := e.capturedLeadWithFields(t, "apollo", "a-1", capture.LeadFields{
		FullName: "Jonas Petersen",
		Email:    "jonas@nordwind.test",
		Title:    "Head of Operations",
	})
	e.capturedLeadWithFields(t, "legacy_crm", "h-9", capture.LeadFields{
		FullName:    "J. Petersen",
		Email:       "jonas@nordwind.test",
		CompanyName: "Nordwind Logistik",
		Title:       "Operations Lead",
	})

	change := e.stagedChange(t, incumbent)
	for field, want := range map[string]string{
		// What the message said. Capture's own tagless spelling, which is what
		// the accept folds from and what every pending row carries.
		"FullName":    "J. Petersen",
		"CompanyName": "Nordwind Logistik",
		"Title":       "Operations Lead",
		// What the lead holds for each of them — including the empty company
		// name, which is the one field this accept will actually fill.
		"current_full_name":    "Jonas Petersen",
		"current_company_name": "",
		"current_title":        "Head of Operations",
	} {
		if got := change[field]; got != want {
			t.Errorf("the staged card's %s is %v, want %q — the reader cannot tell which "+
				"captured values the accept will drop", field, got, want)
		}
	}
}
