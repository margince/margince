// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An active opt-out never expires, and deleting old proof never resets anyone
// to "permission unknown". This drives the real retention pass with every
// authorable scope set to its most destructive action, then asks the real gate
// about a subject who comes back on a new record.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The three ways a subject's stop is held, each on its own aged record:
//   - a contact-wide objection on a contact with two addresses;
//   - a marketing withdrawal with no stop at all;
//   - a lead whose own address is gone but whose stop names one.
func TestEveryRetentionRunKeepsEveryActiveStop(t *testing.T) {
	e := integration.Setup(t)
	store := consent.NewStore(InstallationDB(e.Pool))
	gate := consent.NewGate(store)

	objector := seedAgedContact(t, e, "Two Addresses", "first@stops.test", "second@stops.test")
	if err := store.Suppress(e.Admin(), consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](objector), Kind: commsauthz.ReasonObjection,
		Reason: "asked on the phone to stop the newsletter",
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}
	withdrawer := seedAgedContact(t, e, "Withdrew", "withdrew@stops.test")
	withdrawMarketing(t, e, store, withdrawer)
	lead := seedLeadWithPinnedStop(t, e, "lead-stop@stops.test")

	setEveryScopeToItsMostDestructiveAction(t, e)
	integration.SettleIntoInstall(t, e)
	svc := NewRetentionServiceFor(InstallationDB(e.Pool), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(principal.SystemActing(e.Admin(), "system")); err != nil {
		t.Fatalf("running the retention pass: %v", err)
	}
	assertAnonymized(t, e, objector)
	assertAnonymized(t, e, withdrawer)
	assertLeadAnonymized(t, e, lead)

	for _, address := range []string{"first@stops.test", "second@stops.test", "withdrew@stops.test", "lead-stop@stops.test"} {
		seedContactWith(t, e, "Came Back", address)
		set, err := gate.Preview(e.Admin(), commsauthz.Request{
			Recipients: []connector.Recipient{{Email: address}},
			Context:    commsauthz.CategoryMarketing,
		})
		if err != nil {
			t.Fatalf("asking the gate about %s: %v", address, err)
		}
		if len(set.Decisions) != 1 || set.Decisions[0].Suppression != commsauthz.ReasonObjection {
			t.Errorf("marketing to %s after retention = %+v, want refused by the objection the subject made "+
				"before their old record was anonymized", address, set.Decisions)
		}
	}
}

func seedAgedContact(t *testing.T, e *integration.Env, name string, addresses ...string) ids.UUID {
	t.Helper()
	id := seedContactWith(t, e, name, addresses...)
	ageRow(t, e, `UPDATE contact SET created_at = now() - interval '30 days' WHERE id = $1`, id)
	return id
}

func seedContactWith(t *testing.T, e *integration.Env, name string, addresses ...string) ids.UUID {
	t.Helper()
	emails := make([]contacts.ContactEmailInput, 0, len(addresses))
	for i, address := range addresses {
		emails = append(emails, contacts.ContactEmailInput{Email: address, EmailType: "work", IsPrimary: i == 0, Position: i})
	}
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual", Emails: emails,
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.UUID(created.Id)
}

// withdrawMarketing withdraws a marketing purpose through the store a rep's
// withdrawal is recorded by.
func withdrawMarketing(t *testing.T, e *integration.Env, store *consent.Store, contact ids.UUID) {
	t.Helper()
	purpose, err := store.CreatePurpose(e.Admin(), "retention_newsletter", "Newsletter", false)
	if err != nil {
		t.Fatalf("creating a marketing purpose: %v", err)
	}
	if _, err := store.Record(e.Admin(), consent.RecordInput{
		ContactID: ids.From[ids.ContactKind](contact), PurposeID: purpose.ID, NewState: "withdrawn",
	}); err != nil {
		t.Fatalf("withdrawing marketing: %v", err)
	}
}

// seedLeadWithPinnedStop is a lead whose own address is already empty, holding
// a stop that names the address it was pressed from: the shape a one-click
// unsubscribe leaves on a lead.
func seedLeadWithPinnedStop(t *testing.T, e *integration.Env, address string) ids.UUID {
	t.Helper()
	var lead ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `
			INSERT INTO lead (full_name, status, source, captured_by, created_at)
			VALUES ('Pressed Unsubscribe', 'new', 'inbound', 'human:test', now() - interval '30 days')
			RETURNING id`).Scan(&lead); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO communication_suppression (lead_id, address, kind, source, captured_by, decided_by_level)
			VALUES ($1, $2, $3, 'public_link', 'system', 'machine')`, lead, address, commsauthz.ReasonObjection)
		return err
	}); err != nil {
		t.Fatalf("seeding the lead's stop: %v", err)
	}
	return lead
}

// setEveryScopeToItsMostDestructiveAction derives the scopes from the engine's
// own vocabulary, so a scope added later is in this run without an edit here.
func setEveryScopeToItsMostDestructiveAction(t *testing.T, e *integration.Env) {
	t.Helper()
	scopes := privacy.AuthorableScopes()
	if len(scopes) < 8 {
		t.Fatalf("read %d retention scopes, want every selector the engine has", len(scopes))
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		for _, wire := range scopes {
			scope, err := privacy.ParseRetentionScope(wire)
			if err != nil {
				return err
			}
			actions := privacy.ActionsForScope(scope.ObjectType)
			if _, err := tx.Exec(context.Background(), `
				INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis, enabled)
				VALUES ($1, $2, 1, $3, 'storage_limitation', true)
				ON CONFLICT ON CONSTRAINT retention_policy_unique
				DO UPDATE SET retain_days = 1, action = EXCLUDED.action, enabled = true`,
				scope.ObjectType, scope.CategoryPtr(), actions[0]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("setting every retention scope: %v", err)
	}
}

func ageRow(t *testing.T, e *integration.Env, statement string, id ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), statement, id)
		return err
	}); err != nil {
		t.Fatalf("ageing the fixture: %v", err)
	}
}

// assertAnonymized proves the pass reached the record, so the gate's answer
// below is about a stop that outlived its record and not one never touched.
func assertAnonymized(t *testing.T, e *integration.Env, contact ids.UUID) {
	t.Helper()
	var addresses int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM contact_email WHERE contact_id = $1`, contact).Scan(&addresses)
	}); err != nil {
		t.Fatalf("reading the anonymized contact: %v", err)
	}
	if addresses != 0 {
		t.Fatalf("contact %s still holds %d address(es): the retention pass did not anonymize it", contact, addresses)
	}
}

func assertLeadAnonymized(t *testing.T, e *integration.Env, lead ids.UUID) {
	t.Helper()
	var name string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT full_name FROM lead WHERE id = $1`, lead).Scan(&name)
	}); err != nil {
		t.Fatalf("reading the anonymized lead: %v", err)
	}
	if name != "Anonymized Lead" {
		t.Fatalf("lead %s is still named %q: the retention pass did not anonymize it", lead, name)
	}
}
