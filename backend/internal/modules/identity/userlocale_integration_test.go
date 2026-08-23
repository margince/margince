// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A member choosing their own display language. The properties worth a real
// database: the write is self-scoped, it carries the audit and outbox rows every
// mutation owes, re-choosing writes nothing, and a language the product ships no
// catalog for is refused.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// seatChoosingALanguage is one bootstrapped installation and a context acting
// as its admin — a seat created by the real bootstrap writer rather than by an
// INSERT of this file's own, so what is written here is what production writes.
func seatChoosingALanguage(t *testing.T) (*Service, context.Context, ids.UUID) {
	t.Helper()
	_, pool := setupIdentityDB(t)
	svc := NewService(pool)
	name := "locale-" + ids.NewV7().String()
	wsID, _, _, err := svc.BootstrapInstallation(context.Background(), func() (InstallationBootstrap, error) {
		return InstallationBootstrap{
			OrganizationName: name,
			BaseCurrency:     "EUR",
			BaseLanguage:     "en",
			Timezone:         "Europe/Berlin",
			AdminEmail:       "admin@" + name + ".test",
			AdminName:        "Admin",
			AdminPassword:    "a bootstrap password!",
		}, nil
	}, nil)
	if err != nil {
		t.Fatalf("bootstrapping an installation to hold the seat: %v", err)
	}
	var userID ids.UUID
	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM app_user WHERE email = $1`, "admin@"+name+".test").Scan(&userID)
	}); err != nil {
		t.Fatalf("reading the bootstrapped admin: %v", err)
	}
	ctx := principal.WithWorkspaceID(context.Background(), wsID.UUID)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + userID.String(), UserID: userID,
	})
	return svc, ctx, userID
}

// localeLedger counts the rows a locale write is supposed to leave behind.
func localeLedger(t *testing.T, svc *Service, userID ids.UUID) (audits, events int) {
	t.Helper()
	ctx := context.Background()
	if err := svc.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM audit_log
			  WHERE entity_type = 'user' AND entity_id = $1
			    AND after ? 'locale'`, userID).Scan(&audits); err != nil {
			return err
		}
		// The type lives inside the envelope: the outbox row carries a stream
		// and a JSON envelope, not a typed column.
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM event_outbox
			  WHERE envelope ->> 'type' = 'user_locale.changed'`).
			Scan(&events)
	}); err != nil {
		t.Fatal(err)
	}
	return audits, events
}

func TestChoosingADisplayLanguageWritesTheSeatItsAuditAndItsEvent(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)

	seat, err := svc.SaveMyLocale(ctx, "de")
	if err != nil {
		t.Fatalf("choosing German: %v", err)
	}
	if seat.Locale != "de" {
		t.Errorf("the answer reports locale %q, want de", seat.Locale)
	}
	if seat.UserID.UUID != userID {
		t.Errorf("the answer names user %s, want the caller %s", seat.UserID, userID)
	}

	audits, events := localeLedger(t, svc, userID)
	if audits != 1 {
		t.Errorf("audit rows naming a locale = %d, want 1 — a change to a seat is answerable later", audits)
	}
	if events != 1 {
		t.Errorf("user_locale.changed events = %d, want 1 — the write shape owes an outbox row", events)
	}

	// Re-choosing the SAME language is not a change. A settings page that saves
	// on every render would otherwise fill the ledger with a change nobody made.
	if _, err := svc.SaveMyLocale(ctx, "de"); err != nil {
		t.Fatalf("re-asserting German: %v", err)
	}
	againAudits, againEvents := localeLedger(t, svc, userID)
	if againAudits != audits || againEvents != events {
		t.Errorf("re-choosing the same language wrote %d audits / %d events, want the %d / %d already there",
			againAudits, againEvents, audits, events)
	}
}

func TestADisplayLanguageTheProductCannotRenderIsRefused(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)

	// Each of these would leave the interface reaching for a catalog that does
	// not exist, which renders as raw message keys.
	for _, bad := range []string{"", "EN", "en-GB", "fr"} {
		_, err := svc.SaveMyLocale(ctx, bad)
		if err == nil {
			t.Errorf("locale %q was accepted; the product ships no catalog for it", bad)
			continue
		}
		var fault apperrors.FieldFault
		if !errors.As(err, &fault) {
			t.Errorf("refusing %q gave %T, which names no field for the form to attach it to", bad, err)
			continue
		}
		if field, _, _ := fault.FieldFault(); field != "locale" {
			t.Errorf("refusing %q named field %q, want locale", bad, field)
		}
	}
	// And nothing was written on the way through.
	if audits, events := localeLedger(t, svc, userID); audits != 0 || events != 0 {
		t.Errorf("a refused locale left %d audits / %d events behind, want none", audits, events)
	}
}

func TestOnlyAHumanSeatChoosesADisplayLanguage(t *testing.T) {
	svc, _, _ := seatChoosingALanguage(t)

	// An agent has no interface to render, so it has no display language to
	// choose. The refusal is what keeps this from becoming a way to write
	// somebody else's row from a machine principal.
	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:runner", UserID: ids.NewV7(),
	})
	if _, err := svc.SaveMyLocale(agent, "de"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent choosing a display language got %v, want ErrPermissionDenied", err)
	}
}
