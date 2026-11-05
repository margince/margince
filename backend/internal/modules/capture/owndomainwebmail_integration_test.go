// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// Webmail among the installation's own domains, over a real Postgres.
//
// The registry vets a domain as it is written. The own company's claims reach
// the same sets unvetted, so the sets are filtered where they are read.
//
// A shared webmail domain there is quiet and expensive. Capture reads every
// message between two of its users as internal, and skipped mail is not offered
// again.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// trustedDomainsOf reads the vouched-for set of a workspace whose own company
// claims these domains.
func trustedDomainsOf(t *testing.T, claims ...string) capture.InternalDomains {
	t.Helper()
	ctx, db := bootstrapInternalMailWorkspace(t, claims...)
	var trusted capture.InternalDomains
	if err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		var err error
		trusted, err = capture.TrustedOwnDomainsTx(ctx, tx)
		return err
	}); err != nil {
		t.Fatalf("reading the trusted own domains: %v", err)
	}
	return trusted
}

// settingsAdmin is the seat that keeps the workspace's consumer-mail list. The
// row is seeded because the list records who changed it.
func settingsAdmin(ctx context.Context, t *testing.T, db *database.DB) context.Context {
	t.Helper()
	seat := ids.NewV7()
	if err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO app_user (id, email, display_name, status)
			VALUES ($1, $2, 'Settings Admin', 'active')`,
			seat, "settings-"+seat.String()+"@example.test")
		return err
	}); err != nil {
		t.Fatalf("seeding the settings seat: %v", err)
	}
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:settings", UserID: seat,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"capture_settings": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func TestWebmailClaimedByTheAnchorCompanyIsNotOneOfOurDomains(t *testing.T) {
	trusted := trustedDomainsOf(t, "margince.test", "gmail.com")

	if trusted.Covers("somebody@gmail.com") {
		t.Error("a webmail address the anchor company's claim brought in reads as ours")
	}
	// The company's real domain still counts, so the filter drops the webmail
	// claim rather than the claim that carried it.
	if !trusted.Covers("anna@margince.test") {
		t.Error("the filter stopped the company's own domain counting")
	}
}

// An administrator keeps a domain the baseline calls consumer mail by filing a
// `never` entry. That entry has to win here too.
func TestAnAdministratorsNeverEntryKeepsTheDomainOurs(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "gmail.com")
	// Written through the store an operator writes through, which normalizes
	// the domain to the registrable form the matcher keys on.
	if _, err := capture.NewFreemailDomains(db).Add(
		settingsAdmin(ctx, t, db), "gmail.com", capture.FreemailKindNever); err != nil {
		t.Fatalf("recording the never entry: %v", err)
	}

	var trusted capture.InternalDomains
	if err := database.WithWorkspaceTx(ctx, db.Pool(), func(tx pgx.Tx) error {
		var err error
		trusted, err = capture.TrustedOwnDomainsTx(ctx, tx)
		return err
	}); err != nil {
		t.Fatalf("reading the trusted own domains: %v", err)
	}
	if !trusted.Covers("somebody@gmail.com") {
		t.Error("a never entry did not keep the domain among ours")
	}
}
