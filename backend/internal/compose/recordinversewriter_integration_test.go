// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The writers' own refusals: what an undo's write refuses on its own, inside
// its transaction, whatever the decision before it answered.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// auditRowOf reads one audit entry the way the restore route does.
func auditRowOf(t *testing.T, e *integration.Env, auditID ids.UUID) AuditRow {
	t.Helper()
	var row AuditRow
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT id, entity_type, entity_id, action, before, after, evidence, occurred_at
			  FROM audit_log WHERE id = $1`, auditID).
			Scan(&row.ID, &row.EntityType, &row.EntityID, &row.Action, &row.Before, &row.After, &row.Evidence, &row.OccurredAt)
	}); err != nil {
		t.Fatalf("read entry %s: %v", auditID, err)
	}
	return row
}

// The archive an undo writes asks about a colleague's work again, in its own
// transaction, and refuses without archiving.
func TestTheArchiveAnUndoWritesRefusesAColleaguesWorkByItself(t *testing.T) {
	e := integration.Setup(t)
	created, err := e.Contacts.CreateContact(machineCtx(e), contacts.CreateContactInput{FullName: "Imported Ida", Source: "import"})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.UUID(created.Id)
	row := auditRowOf(t, e, latestAuditRowID(t, e, "contact", contact, actionCreate))
	taggedAndListed(t, e, "contact", contact)

	err = restoreSeamFor(e).inverses.perform(e.Admin(), e.Pool, row, inverseArchive, currentVersion(t, e, "contact", contact))
	if reason := refusedFor(t, inverseWriteRefusal(err)); reason != ReasonSuperseded {
		t.Errorf("the archive refused %q, want %q", reason, ReasonSuperseded)
	}
	if isArchived(t, e, "contact", contact) {
		t.Error("the refused archive committed")
	}
}

// An archive's own cascade names the tags and list memberships it removed, so
// what a colleague gave the record is visible after the archive deleted it.
func TestAnArchivesCascadeNamesTheColleagueWorkItDropped(t *testing.T) {
	e := integration.Setup(t)
	tagged := e.SeedContact(t, "Tagged Tom", nil)
	taggedAndListed(t, e, "contact", tagged)
	plain := e.SeedContact(t, "Plain Pia", nil)
	for _, contact := range []ids.UUID{tagged, plain} {
		if _, err := NewProvider(e.Pool).Archive(e.Admin(), datasource.EntityRef{Type: "contact", ID: contact}); err != nil {
			t.Fatal(err)
		}
	}
	dropped := func(contact ids.UUID) bool {
		var out bool
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = contacts.ArchiveDroppedColleagueWork(context.Background(), tx, "contact", contact)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if !dropped(tagged) {
		t.Error("the archive of a contact a colleague tagged and listed names no colleague work")
	}
	if dropped(plain) {
		t.Error("the archive of an untouched contact names colleague work")
	}
}

// A demotion asked to spare a colleague's work refuses when the contact the
// promotion created carries a tag a colleague gave it.
func TestADemotionRefusesWhenAColleagueWorkedOnTheCreatedContact(t *testing.T) {
	e := integration.Setup(t)
	name, email := "Lena Lead", "lena@prospect.test"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "import", FullName: &name, Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.UUID(lead.Id)
	contact, _, err := e.Contacts.PromoteLead(machineCtx(e), ids.From[ids.LeadKind](leadID), contacts.PromoteLeadInput{Trigger: "inbound_reply"})
	if err != nil {
		t.Fatal(err)
	}
	promote := auditRowOf(t, e, latestAuditRowID(t, e, entityTypeLead, leadID, actionPromote))
	taggedAndListed(t, e, "contact", ids.UUID(contact.Id))

	_, err = e.Contacts.DemoteLead(e.Admin(), ids.From[ids.LeadKind](leadID), demoteReason,
		contacts.NotWorkedOnByAColleagueSince(promote.OccurredAt, promote.ID))
	var touched *contacts.HumanTouchedError
	if !errors.As(err, &touched) {
		t.Errorf("the demotion answered %v, want a refusal naming the colleague's work", err)
	}
	if isArchived(t, e, "contact", ids.UUID(contact.Id)) {
		t.Error("the refused demotion archived the contact")
	}
}
