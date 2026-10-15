// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An undone import frees the source key its lead held, so the same file lands
// the same leads again instead of replaying the retired row.
func TestUndoFreesTheSourceKeyOfAnImportedLead(t *testing.T) {
	e := integration.Setup(t)
	system, ref, name := "csv:undo-key", "lead-1", "Imported Lead"
	create := func() (ids.UUID, bool) {
		lead, created, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
			FullName: &name, Status: "new", Source: "import",
			SourceSystem: &system, SourceID: &ref,
		})
		if err != nil {
			t.Fatalf("creating the lead: %v", err)
		}
		return ids.UUID(lead.Id), created
	}
	w := &csvWriters{pool: e.DB().Pool(), contacts: e.Contacts}
	first, created := create()
	if !created {
		t.Fatal("first import did not create the lead")
	}
	if err := w.Reverse(e.Admin(), migration.ObjectLead, first, clocktest.Now(t).Add(time.Hour)); err != nil {
		t.Fatalf("undo: %v", err)
	}
	second, created := create()
	if !created || second == first {
		t.Fatalf("second import created=%v id=%v (first %v); want a new lead", created, second, first)
	}
}

// A lead a colleague edited after the import stays, with its key.
func TestUndoKeepsAHumanEditedLeadAndItsSourceKey(t *testing.T) {
	e := integration.Setup(t)
	system, ref, name := "csv:undo-kept", "lead-1", "Edited Lead"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: &name, Status: "new", Source: "import", SourceSystem: &system, SourceID: &ref,
	})
	if err != nil {
		t.Fatalf("creating the lead: %v", err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	title := "Head of Sales"
	if _, err := e.Contacts.UpdateLead(e.Admin(), id, contacts.UpdateLeadInput{Title: &title}); err != nil {
		t.Fatalf("editing the lead: %v", err)
	}
	w := &csvWriters{pool: e.DB().Pool(), contacts: e.Contacts}
	err = w.Reverse(e.Admin(), migration.ObjectLead, ids.UUID(lead.Id), clocktest.Now(t).Add(-time.Hour))
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("undo of an edited lead = %v, want a conflict", err)
	}
	got, err := e.Contacts.GetLead(e.Admin(), id, storekit.IncludeArchived)
	if err != nil || got.ArchivedAt != nil || got.SourceId == nil || *got.SourceId != ref {
		t.Fatalf("lead after refused undo = %+v, %v; want live with its key", got, err)
	}
}
