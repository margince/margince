// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// A zone the database cannot read is refused as an invalid installation zone,
// so it never reaches the SQL every activity write runs. One it reads passes.
func TestAZoneTheDatabaseLacksIsRefused(t *testing.T) {
	t.Parallel()
	_, pool := setupIdentityDB(t)
	ctx := context.Background()
	for zone, refused := range map[string]bool{"Margince/Nowhere": true, "Pacific/Kiritimati": false} {
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return refuseZoneTheDatabaseLacksTx(ctx, tx, zone) })
		var invalid settings.InvalidValue
		switch {
		case refused && (!errors.As(err, &invalid) || invalid.Setting != Timezone.Key()):
			t.Errorf("%s: the check answered %v, want a refusal naming %s", zone, err, Timezone.Key())
		case !refused && err != nil:
			t.Errorf("%s: the check refused a zone the database reads: %v", zone, err)
		}
	}
}

// A claim that sends a zone Go or the database does not know succeeds, and
// the installation and its first admin are stored in UTC.
func TestAClaimWithAZoneTheServerDoesNotKnowStoresUTC(t *testing.T) {
	for name, sent := range map[string]string{
		"a name neither engine knows": "Margince/Nowhere",
		"a rule only Postgres reads":  "XYZ-3",
	} {
		t.Run(name, func(t *testing.T) {
			svc := newSetupService(t)
			ctx := context.Background()
			token, err := svc.MintSetupToken(ctx)
			if err != nil {
				t.Fatalf("minting: %v", err)
			}
			in := claimInput("zoned")
			in.Timezone = sent
			if _, _, err := svc.ClaimInstallation(ctx, token, in, nil); err != nil {
				t.Fatalf("a claim sending %q was refused: %v", sent, err)
			}
			var installation, admin string
			if err := pgx.BeginFunc(ctx, svc.db.Pool(), func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `
					SELECT (SELECT value #>> '{}' FROM setting WHERE key = $1),
					       (SELECT timezone FROM app_user WHERE email = $2)`,
					Timezone.Key(), in.AdminEmail).Scan(&installation, &admin)
			}); err != nil {
				t.Fatalf("reading the stored zones: %v", err)
			}
			if installation != "UTC" || admin != "UTC" {
				t.Errorf("the claim stored %q for the installation and %q for its admin, want UTC for both", installation, admin)
			}
		})
	}
}

// A malformed zone is still refused: UTC stands in for an unknown name only.
func TestAClaimWithAMalformedZoneIsRefused(t *testing.T) {
	svc := newSetupService(t)
	ctx := context.Background()
	token, err := svc.MintSetupToken(ctx)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	in := claimInput("malformed")
	in.Timezone = "Local"
	if _, _, err := svc.ClaimInstallation(ctx, token, in, nil); err == nil {
		t.Error("a claim sending the zone Local was accepted")
	}
}
