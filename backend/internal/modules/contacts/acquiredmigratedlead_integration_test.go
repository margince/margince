// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// An imported lead becomes a migrated contact only through conversion. Mail
// stays on the lead, and the resulting contact is owed nothing.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedLead writes the lead and import audit row. The original address may
// differ from the current one after an edit.
func (e *dedupeEnv) seedLead(ctx context.Context, t *testing.T, email, createdWith, sourceSystem string) ids.LeadID {
	t.Helper()
	id := ids.NewV7()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO lead (id, full_name, email, status, source, source_system, source_id, captured_by)
			VALUES ($1, 'Seeded Lead', $2, 'new', 'seed', $3, $4, 'human:seed')`,
			id, email, sourceSystem, id.String()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
			VALUES ('human', 'seed', 'create', 'lead', $1, jsonb_build_object('email', $2::text))`,
			id, createdWith)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.LeadKind](id)
}

// convertAfterMail captures mail without making a contact, then converts the
// lead. The conversion is the only contact-producing step.
func (e *dedupeEnv) convertAfterMail(ctx context.Context, t *testing.T, lead ids.LeadID, mail EnsureCounterpartyInput) ids.ContactID {
	t.Helper()
	res, err := e.store.EnsureCounterparty(ctx, mail)
	if err != nil || res.ContactCreated || res.LeadID != lead {
		t.Fatalf("ensure = %+v (err %v), want the mail filed on lead %v and no contact", res, err, lead)
	}
	contact, _, err := e.store.PromoteLead(ctx, lead, PromoteLeadInput{Trigger: string(TriggerHumanQualify)})
	if err != nil {
		t.Fatalf("converting the lead: %v", err)
	}
	return ids.From[ids.ContactKind](ids.UUID(contact.Id))
}

// outboundOnly turns a seeded message round: we sent it, and they only
// received it — the case that is otherwise recorded as unknown.
func (e *dedupeEnv) outboundOnly(ctx context.Context, t *testing.T, in EnsureCounterpartyInput) EnsureCounterpartyInput {
	t.Helper()
	in.Replied = false
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE activity SET direction = 'outbound' WHERE id = $1`, in.ActivityID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE activity_participant SET role = 'to' WHERE activity_id = $1`, in.ActivityID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestAnAddressAMigratedLeadHoldsIsACRMMigration(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead := e.seedLead(ctx, t, "buyer@oldcrm.test", "buyer@oldcrm.test", "mirror:hubspot")
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, "buyer@oldcrm.test", "oldcrm.test", clocktest.Now(t).Add(-time.Hour)))

	contact := e.convertAfterMail(ctx, t, lead, in)
	if kind, _ := acquisitionOf(ctx, t, e.store, contact); kind != AcquiredCRMMigration {
		t.Errorf("a counterparty the old CRM held as a lead is recorded as %q, want %q", kind, AcquiredCRMMigration)
	}
}

// A lead somebody typed in here is not a migration, and says nothing about how
// the address was obtained.
func TestALeadEnteredHereIsNotAMigration(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead := e.seedLead(ctx, t, "walkin@fair.test", "walkin@fair.test", "web_form")
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, "walkin@fair.test", "fair.test", clocktest.Now(t).Add(-time.Hour)))

	contact := e.convertAfterMail(ctx, t, lead, in)
	if kind, _ := acquisitionOf(ctx, t, e.store, contact); kind != AcquiredUnknownLegacy {
		t.Errorf("a lead entered here made the contact %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// An imported lead whose email was changed after the import does not make a
// stranger a migrated contact. Anyone who may edit leads may change an email;
// only an importer may write the mirror: prefix, and the two together must not
// excuse a notice for somebody the old CRM never held.
func TestAnImportedLeadRetargetedAtAnotherAddressIsNotAMigration(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead := e.seedLead(ctx, t, "stranger@elsewhere.test", "original@oldcrm.test", "mirror:hubspot")
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, "stranger@elsewhere.test", "elsewhere.test", clocktest.Now(t).Add(-time.Hour)))

	contact := e.convertAfterMail(ctx, t, lead, in)
	if kind, _ := acquisitionOf(ctx, t, e.store, contact); kind != AcquiredUnknownLegacy {
		t.Errorf("a lead retargeted after the import made the contact %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// A lead who wrote to us converts into a contact who contacted us first. The
// mail is the evidence a capture would have read.
func TestALeadWhoWroteConvertsIntoAContactWhoWrote(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead := e.seedLead(ctx, t, "asked@fair.test", "asked@fair.test", "web_form")
	in := e.datedEnsureInput(ctx, t, "asked@fair.test", "fair.test", clocktest.Now(t).Add(-time.Hour))

	contact := e.convertAfterMail(ctx, t, lead, in)
	if kind, _ := acquisitionOf(ctx, t, e.store, contact); kind != AcquiredSubjectInitiated {
		t.Errorf("a lead who wrote to us made the contact %q, want %q", kind, AcquiredSubjectInitiated)
	}
}
