// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The name a member's colleagues greet them by: the member sets and clears it,
// and their first federated sign-in fills it from the provider's given name
// only while it is empty.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedGreetingName reads the column itself, which is what the intro draft
// and /me read, rather than the answer a write returned.
func storedGreetingName(t *testing.T, svc *Service, userID ids.UUID) *string {
	t.Helper()
	var name *string
	ctx := context.Background()
	if err := svc.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT greeting_name FROM app_user WHERE id = $1`, userID).Scan(&name)
	}); err != nil {
		t.Fatal(err)
	}
	return name
}

// greetingNameLedger counts the audit and outbox rows greeting-name changes
// left for one member.
func greetingNameLedger(t *testing.T, svc *Service, userID ids.UUID) (audits, events int) {
	t.Helper()
	ctx := context.Background()
	if err := svc.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM audit_log
			  WHERE entity_type = 'user' AND entity_id = $1 AND action = 'update'
			    AND after ? 'greeting_name'`, userID).Scan(&audits); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM event_outbox
			  WHERE envelope ->> 'type' = 'user_greeting_name.changed'
			    AND envelope -> 'entity' ->> 'id' = $1::text`, userID).Scan(&events)
	}); err != nil {
		t.Fatal(err)
	}
	return audits, events
}

func textOf(name *string) string {
	if name == nil {
		return "<null>"
	}
	return *name
}

func TestAMemberSetsAndClearsTheNameTheyAreGreetedBy(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)

	typed := "  Sofia  "
	seat, err := svc.SaveMyGreetingName(ctx, &typed)
	if err != nil {
		t.Fatalf("setting the greeting name: %v", err)
	}
	if textOf(seat.GreetingName) != "Sofia" {
		t.Errorf("the answer reports %q, want the trimmed Sofia", textOf(seat.GreetingName))
	}
	if got := storedGreetingName(t, svc, userID); textOf(got) != "Sofia" {
		t.Errorf("app_user holds %q, want Sofia", textOf(got))
	}
	if audits, events := greetingNameLedger(t, svc, userID); audits != 1 || events != 1 {
		t.Errorf("setting wrote %d audits / %d events, want 1 / 1", audits, events)
	}

	// The same name again moves nothing and writes nothing.
	if _, err := svc.SaveMyGreetingName(ctx, &typed); err != nil {
		t.Fatalf("re-saving the same name: %v", err)
	}
	if audits, events := greetingNameLedger(t, svc, userID); audits != 1 || events != 1 {
		t.Errorf("re-saving the same name wrote %d audits / %d events, want the 1 / 1 already there", audits, events)
	}

	// Blank clears, to NULL rather than "", so greetings fall back to the
	// display name's first word.
	blank := "   "
	cleared, err := svc.SaveMyGreetingName(ctx, &blank)
	if err != nil {
		t.Fatalf("clearing the greeting name: %v", err)
	}
	if cleared.GreetingName != nil {
		t.Errorf("the answer reports %q after clearing, want null", textOf(cleared.GreetingName))
	}
	if got := storedGreetingName(t, svc, userID); got != nil {
		t.Errorf("app_user holds %q after clearing, want NULL", textOf(got))
	}
	if audits, events := greetingNameLedger(t, svc, userID); audits != 2 || events != 2 {
		t.Errorf("clearing wrote %d audits / %d events in all, want 2 / 2", audits, events)
	}
}

func TestAnOverlongGreetingNameIsRefusedOnItsField(t *testing.T) {
	svc, ctx, userID := seatChoosingALanguage(t)

	// Counted in runes: 101 two-byte characters is past the bound, 100 is not.
	over := strings.Repeat("ü", greetingNameMaxRunes+1)
	_, err := svc.SaveMyGreetingName(ctx, &over)
	var fault apperrors.FieldFault
	if !errors.As(err, &fault) {
		t.Fatalf("err = %v, want a field fault", err)
	}
	if field, _, _ := fault.FieldFault(); field != "greeting_name" {
		t.Errorf("the refusal points at %q, want greeting_name", field)
	}
	if got := storedGreetingName(t, svc, userID); got != nil {
		t.Errorf("a refused name was stored: %q", textOf(got))
	}
	exact := strings.Repeat("ü", greetingNameMaxRunes)
	if _, err := svc.SaveMyGreetingName(ctx, &exact); err != nil {
		t.Errorf("a name at the bound was refused: %v", err)
	}
}

func TestTheFirstFederatedSignInFillsAnEmptyGreetingName(t *testing.T) {
	svc, _, userID, email := seedSSOEnv(t, "sso-greeting-fill")

	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google",
		OIDCClaims{Subject: "sub-greeting", Email: email, GivenName: " Carol "}); err != nil {
		t.Fatalf("LoginViaFederatedIdentity: %v", err)
	}
	if got := storedGreetingName(t, svc, userID.UUID); textOf(got) != "Carol" {
		t.Errorf("greeting_name = %q after the first sign-in, want the provider's given name Carol", textOf(got))
	}
	if audits, events := greetingNameLedger(t, svc, userID.UUID); audits != 1 || events != 1 {
		t.Errorf("the fill wrote %d audits / %d events, want 1 / 1", audits, events)
	}
}

func TestASignInNeverReplacesAGreetingNameAlreadySet(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-greeting-kept")
	if _, err := conn.Exec(context.Background(),
		`UPDATE app_user SET greeting_name = 'Caz' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google",
		OIDCClaims{Subject: "sub-greeting-kept", Email: email, GivenName: "Carol"}); err != nil {
		t.Fatalf("LoginViaFederatedIdentity: %v", err)
	}
	if got := storedGreetingName(t, svc, userID.UUID); textOf(got) != "Caz" {
		t.Errorf("greeting_name = %q after sign-in, want the member's own Caz kept", textOf(got))
	}
	if audits, events := greetingNameLedger(t, svc, userID.UUID); audits != 0 || events != 0 {
		t.Errorf("a sign-in that changed nothing wrote %d audits / %d events, want none", audits, events)
	}
}

// A second provider linked later is not a first sign-in: a name the member
// cleared after the first one stays cleared.
func TestASecondProviderDoesNotRefillAClearedGreetingName(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-greeting-second")
	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "google",
		OIDCClaims{Subject: "sub-greeting-google", Email: email, GivenName: "Carol"}); err != nil {
		t.Fatalf("the Google sign-in: %v", err)
	}
	if _, err := conn.Exec(context.Background(),
		`UPDATE app_user SET greeting_name = NULL WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.LoginViaFederatedIdentity(groupSyncCtx(), "microsoft",
		OIDCClaims{Subject: "sub-greeting-microsoft", Email: email, GivenName: "Carol"}); err != nil {
		t.Fatalf("the Microsoft sign-in: %v", err)
	}
	if got := storedGreetingName(t, svc, userID.UUID); got != nil {
		t.Errorf("greeting_name = %q after a second provider's first sign-in, want the member's clear kept", textOf(got))
	}
}
