// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// countryTestPack makes "zz" a compiled-in code in this test binary, so a
// seed naming it passes the validator and the test reaches the stored-row rule.
type countryTestPack struct{}

func (countryTestPack) Code() jurisdiction.Code { return "zz" }

//nolint:ireturn // jurisdiction.Pack declares this method returning the interface.
func (countryTestPack) Retention() jurisdiction.Retention { return nil }

func init() { jurisdiction.Register(countryTestPack{}) }

// The claim form asks for a jurisdiction, so a claim that sends none is a
// client that stopped asking. It is refused and the token stays usable.
func TestAClaimNamingNoJurisdictionIsRefused(t *testing.T) {
	svc := newSetupService(t)
	ctx := context.Background()
	token, err := svc.MintSetupToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	in := claimInput("nocountry")
	in.Country = nil
	_, _, err = svc.ClaimInstallation(ctx, token, in, nil)
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) || invalid.Setting != Country.Key() {
		t.Fatalf("claim without a country = %v, want a refusal naming %s", err, Country.Key())
	}
	if outstanding, err := svc.SetupTokenOutstanding(ctx); err != nil || !outstanding {
		t.Fatalf("the refused claim spent the token (outstanding=%v, err=%v)", outstanding, err)
	}
}

// Choosing no pack is an answer, and it is stored, so a later deployment file
// naming a country does not override what the admin chose.
func TestAClaimChoosingNoPackStoresTheChoice(t *testing.T) {
	_, pool := setupIdentityDB(t)
	svc := NewService(pool)
	ctx := context.Background()
	token, err := svc.MintSetupToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ClaimInstallation(ctx, token, claimInput("nopack"), nil); err != nil {
		t.Fatalf("claim: %v", err)
	}
	stored, err := SeedCountryOnce(ctx, pool, "zz")
	if err != nil {
		t.Fatalf("SeedCountryOnce: %v", err)
	}
	if stored {
		t.Fatal("the file's country replaced the claim's explicit choice of no pack")
	}
	code, err := StoredCountry(ctx, pool)
	if err != nil || code != "" {
		t.Fatalf("StoredCountry = %q, %v; want the claimed empty choice", code, err)
	}
}
