// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func repairLead(t *testing.T, e *integration.Env, name string, email *string) ids.LeadID {
	t.Helper()
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new(name), Email: email, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.LeadKind](ids.UUID(lead.Id))
}

func TestLinkedInIdentityTaskRollsBackAndRefusesAMalformedCursor(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	lead := repairLead(t, e, "Rollback Lead", nil)
	e.WsExec(t, `UPDATE lead SET linkedin_url = 'https://de.linkedin.com/in/rollback' WHERE id = $1`, lead)
	step := linkedInIdentityDataTask().Steps[0]
	rollback := errors.New("roll back after normalization")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Changed != 1 {
			t.Errorf("normalization changed %d rows, want 1", out.Changed)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT linkedin_url FROM lead WHERE id = $1`, lead); got != "https://de.linkedin.com/in/rollback" {
		t.Errorf("rolled-back LinkedIn = %q", got)
	}
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := step.Batch(ctx, tx, datatask.BatchIn{Cursor: []byte(`"not an id"`), Limit: 10})
		return err
	})
	if err == nil {
		t.Fatal("normalization accepted a malformed cursor")
	}
}

func repairContact(t *testing.T, e *integration.Env, name string) ids.ContactID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ContactKind](ids.UUID(contact.Id))
}

func TestLinkedInIdentityTaskNormalizesReadableProfilesAndLeavesOtherValues(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	lead := repairLead(t, e, "Regional Lead", nil)
	contact := repairContact(t, e, "Mobile Contact")
	invalid := repairContact(t, e, "Unclear Contact")
	e.WsExec(t, `UPDATE lead SET linkedin_url = 'https://de.linkedin.com/in/regional/' WHERE id = $1`, lead)
	e.WsExec(t, `INSERT INTO contact_social (contact_id, platform, handle) VALUES
		($1, 'linkedin', 'http://m.linkedin.com/in/mobile/?trk=old'),
		($2, 'linkedin', '@unclear')`, contact, invalid)

	runMachineAddressTask(ctx, t, e, linkedInIdentityDataTask())

	if got := e.WsScalar(t, `SELECT linkedin_url FROM lead WHERE id = $1`, lead); got != "https://www.linkedin.com/in/regional" {
		t.Errorf("lead LinkedIn = %q", got)
	}
	if got := e.WsScalar(t, `SELECT handle FROM contact_social WHERE contact_id = $1 AND platform = 'linkedin'`, contact); got != "https://www.linkedin.com/in/mobile" {
		t.Errorf("contact LinkedIn = %q", got)
	}
	if got := e.WsScalar(t, `SELECT handle FROM contact_social WHERE contact_id = $1 AND platform = 'linkedin'`, invalid); got != "@unclear" {
		t.Errorf("unreadable LinkedIn = %q, want it unchanged", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE actor_id = $1`, "system:datatask:"+linkedInIdentityDataTask().ID); got != 2 {
		t.Errorf("normalization audit rows = %d, want 2", got)
	}

	runMachineAddressTask(ctx, t, e, linkedInIdentityDataTask())
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE actor_id = $1`, "system:datatask:"+linkedInIdentityDataTask().ID); got != 2 {
		t.Errorf("normalization replay left %d audit rows, want 2", got)
	}
}

func TestLeadContactPairTaskMergesEveryUnambiguousPairAfterNormalization(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")

	email := "email-pair@example.test"
	emailLead := repairLead(t, e, "Email Lead", &email)
	emailContact := repairContact(t, e, "Email Contact")
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, email_type, is_primary, position, source, captured_by)
		VALUES ($1, $2, 'work', true, 0, 'manual', 'human:test')`, emailContact, email)

	linkedInLead := repairLead(t, e, "LinkedIn Lead", nil)
	linkedInContact := repairContact(t, e, "LinkedIn Contact")
	e.WsExec(t, `UPDATE lead SET linkedin_url = 'https://de.linkedin.com/in/shared-profile' WHERE id = $1`, linkedInLead)
	e.WsExec(t, `INSERT INTO contact_social (contact_id, platform, handle)
		VALUES ($1, 'linkedin', 'http://m.linkedin.com/in/shared-profile/')`, linkedInContact)

	historicalLead := repairLead(t, e, "Historical Lead", nil)
	historicalContact := repairContact(t, e, "Historical Contact")
	e.WsExec(t, `UPDATE lead SET from_contact_id = $2 WHERE id = $1`, historicalLead, historicalContact)

	reverseLead := repairLead(t, e, "Reverse Lead", nil)
	reverseContact := repairContact(t, e, "Reverse Contact")
	e.WsExec(t, `UPDATE contact SET converted_from_lead_id = $2 WHERE id = $1`, reverseContact, reverseLead)

	ambiguousLead := repairLead(t, e, "Ambiguous Lead", nil)
	ambiguousA := repairContact(t, e, "Ambiguous A")
	ambiguousB := repairContact(t, e, "Ambiguous B")
	e.WsExec(t, `UPDATE lead SET linkedin_url = 'linkedin.com/in/ambiguous' WHERE id = $1`, ambiguousLead)
	e.WsExec(t, `INSERT INTO contact_social (contact_id, platform, handle) VALUES
		($1, 'linkedin', 'https://de.linkedin.com/in/ambiguous'),
		($2, 'linkedin', 'https://m.linkedin.com/in/ambiguous')`, ambiguousA, ambiguousB)

	runMachineAddressTask(ctx, t, e, linkedInIdentityDataTask())
	runMachineAddressTask(ctx, t, e, leadContactPairsDataTask(e.Pool))

	for lead, contact := range map[ids.LeadID]ids.ContactID{
		emailLead: emailContact, linkedInLead: linkedInContact,
		historicalLead: historicalContact, reverseLead: reverseContact,
	} {
		if got := e.WsCount(t, `SELECT count(*) FROM lead
			 WHERE id = $1 AND status = 'promoted' AND archived_at IS NOT NULL AND promoted_contact_id = $2`, lead, contact); got != 1 {
			t.Errorf("lead %s was not promoted into contact %s", lead, contact)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND archived_at IS NULL`, ambiguousLead); got != 1 {
		t.Error("the lead matching two contacts was changed")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'promote'`, "system:datatask:"+leadContactPairsDataTask(e.Pool).ID); got != 4 {
		t.Errorf("pair cleanup promotion audits = %d, want 4", got)
	}

	runMachineAddressTask(ctx, t, e, leadContactPairsDataTask(e.Pool))
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'promote'`, "system:datatask:"+leadContactPairsDataTask(e.Pool).ID); got != 4 {
		t.Errorf("pair cleanup replay left %d promotion audits, want 4", got)
	}
}

func TestLeadContactPairTaskRollsBackWithItsPromotion(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	email := "pair-rollback@example.test"
	lead := repairLead(t, e, "Rollback Pair Lead", &email)
	contact := repairContact(t, e, "Rollback Pair Contact")
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, email_type, is_primary, position, source, captured_by)
		VALUES ($1, $2, 'work', true, 0, 'manual', 'human:test')`, contact, email)
	step := leadContactPairsDataTask(e.Pool).Steps[0]
	rollback := errors.New("roll back after promotion")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Changed != 1 {
			t.Errorf("pair cleanup changed %d rows, want 1", out.Changed)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND archived_at IS NULL`, lead); got != 1 {
		t.Error("a rolled-back cleanup retired the lead")
	}
}
