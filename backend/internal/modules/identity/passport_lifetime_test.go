// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"testing"
	"time"
)

func TestAHandMintedPassportMayNeverExpire(t *testing.T) {
	lifetime, err := lifetimeOf(IssuePassportInput{NeverExpires: true}, false)
	if err != nil {
		t.Fatalf("never expires was refused: %v", err)
	}
	if lifetime != 0 {
		t.Fatalf("lifetime = %v, want none", lifetime)
	}
}

func TestAPassportWithNoLifetimeChosenLastsThirtyDays(t *testing.T) {
	lifetime, err := lifetimeOf(IssuePassportInput{}, false)
	if err != nil || lifetime != 30*24*time.Hour {
		t.Fatalf("lifetime = %v, %v; want 720h", lifetime, err)
	}
}

func TestNeverExpiresIsRefusedBesideAChosenLifetime(t *testing.T) {
	week := 7 * 24 * time.Hour
	_, err := lifetimeOf(IssuePassportInput{NeverExpires: true, TTL: &week}, false)
	var refused *InvalidPassportFieldError
	if !errors.As(err, &refused) || refused.Field != "never_expires" || refused.Code != "never_expires_with_ttl" {
		t.Fatalf("err = %v, want a never_expires_with_ttl refusal", err)
	}
}

func TestAConnectionsPassportAlwaysExpires(t *testing.T) {
	_, err := lifetimeOf(IssuePassportInput{NeverExpires: true}, true)
	var refused *InvalidPassportFieldError
	if !errors.As(err, &refused) || refused.Code != "never_expires_on_connection" {
		t.Fatalf("err = %v, want a never_expires_on_connection refusal", err)
	}
}

func TestALifetimeOverNinetyDaysIsStillRefused(t *testing.T) {
	tooLong := 91 * 24 * time.Hour
	if _, err := lifetimeOf(IssuePassportInput{TTL: &tooLong}, false); err == nil {
		t.Fatal("a 91-day lifetime was accepted")
	}
}
