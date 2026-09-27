// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A person the old CRM held as a lead is a migrated record, whichever door
// later turns them into a contact.
//
// The HubSpot import files a person with no deal and no conversation as a lead.
// When a connected mailbox then finds mail with them, capture mints a contact
// beside the lead. On the first import with a connected mailbox, nine of sixteen
// Art. 14 cases were exactly these people: carried over from HubSpot, and owed
// nothing (consent.DutyFor, "crm_migration").

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *dedupeEnv) seedLead(ctx context.Context, t *testing.T, email, sourceSystem string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO lead (id, full_name, email, status, source, source_system, source_id, captured_by)
			VALUES ($1, 'Seeded Lead', $2, 'new', 'seed', $3, $4, 'human:seed')`,
			ids.NewV7(), email, sourceSystem, ids.NewV7().String())
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
	e.seedLead(ctx, t, "buyer@oldcrm.test", "mirror:hubspot")
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, "buyer@oldcrm.test", "oldcrm.test", time.Now().Add(-time.Hour)))

	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, err)
	}
	if kind, _ := acquisitionOf(ctx, t, e.store, res.ContactID); kind != AcquiredCRMMigration {
		t.Errorf("a person the old CRM held as a lead is recorded as %q, want %q", kind, AcquiredCRMMigration)
	}
}

// A lead somebody typed in here is not a migration, and says nothing about how
// the address was obtained.
func TestALeadEnteredHereIsNotAMigration(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedLead(ctx, t, "walkin@fair.test", "web_form")
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, "walkin@fair.test", "fair.test", time.Now().Add(-time.Hour)))

	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, err)
	}
	if kind, _ := acquisitionOf(ctx, t, e.store, res.ContactID); kind != AcquiredUnknownLegacy {
		t.Errorf("a lead entered here made the contact %q, want %q", kind, AcquiredUnknownLegacy)
	}
}
