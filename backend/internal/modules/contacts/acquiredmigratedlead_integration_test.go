// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A counterparty the old CRM held as a lead is a migrated record, whichever
// door later turns them into a contact.
//
// The HubSpot import files a counterparty with no deal and no conversation as a
// lead. When a connected mailbox then finds mail with them, capture mints a
// contact beside the lead. On the first import with a connected mailbox, nine
// of sixteen Art. 14 cases were exactly these: carried over from HubSpot, and
// owed nothing (consent.DutyFor, "crm_migration").

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedLead writes a lead and the create audit row its import would leave,
// stating the address it was created with — which may differ from the one it
// holds now, the case an edit after the import produces.
func (e *dedupeEnv) seedLead(ctx context.Context, t *testing.T, email, createdWith, sourceSystem string) {
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
	e.seedLead(ctx, t, "buyer@oldcrm.test", "buyer@oldcrm.test", "mirror:hubspot")
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, "buyer@oldcrm.test", "oldcrm.test", time.Now().Add(-time.Hour)))

	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, err)
	}
	if kind, _ := acquisitionOf(ctx, t, e.store, res.ContactID); kind != AcquiredCRMMigration {
		t.Errorf("a counterparty the old CRM held as a lead is recorded as %q, want %q", kind, AcquiredCRMMigration)
	}
}

// A lead somebody typed in here is not a migration, and says nothing about how
// the address was obtained.
func TestALeadEnteredHereIsNotAMigration(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedLead(ctx, t, "walkin@fair.test", "walkin@fair.test", "web_form")
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, "walkin@fair.test", "fair.test", time.Now().Add(-time.Hour)))

	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, err)
	}
	if kind, _ := acquisitionOf(ctx, t, e.store, res.ContactID); kind != AcquiredUnknownLegacy {
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
	e.seedLead(ctx, t, "stranger@elsewhere.test", "original@oldcrm.test", "mirror:hubspot")
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, "stranger@elsewhere.test", "elsewhere.test", time.Now().Add(-time.Hour)))

	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, err)
	}
	if kind, _ := acquisitionOf(ctx, t, e.store, res.ContactID); kind != AcquiredUnknownLegacy {
		t.Errorf("a lead retargeted after the import made the contact %q, want %q", kind, AcquiredUnknownLegacy)
	}
}
