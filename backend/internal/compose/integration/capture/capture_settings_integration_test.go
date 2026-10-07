// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The workspace capture-settings store end to end (CAP-WIRE-7, ADR-0072/A118):
// every role reads the auto-enrich posture; only a holder of the
// capture_settings update grant may change it; the change is an audit-only
// write, and an idempotent no-op writes no audit row.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// integration.BoolPtr addresses a literal for the optional-bool fields the settings
// endpoints take.

// captureSettingsCtx builds a human principal in the env workspace with a
// specific capture_settings grant.
func captureSettingsCtx(e *integration.SearchEnv, grant principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"capture_settings": grant},
			RowScope: principal.RowScopeAll,
		},
	})
}

func TestCaptureSettingsStore(t *testing.T) {
	e := integration.SetupSearch(t)
	store := capturemod.NewSettings(compose.NewSettingsStore(e.Pool))

	admin := captureSettingsCtx(e, principal.ObjectGrant{Read: true, Update: true})
	rep := captureSettingsCtx(e, principal.ObjectGrant{Read: true})
	none := captureSettingsCtx(e, principal.ObjectGrant{})

	auditCount := func() int {
		var n int
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(),
				`SELECT count(*) FROM audit_log WHERE entity_type = 'capture_settings'`).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Default posture is ON (the testing default, migration 0121).
	got, err := store.Get(rep)
	if err != nil {
		t.Fatalf("rep read: %v", err)
	}
	if !got.AutoEnrich {
		t.Fatal("default capture_auto_enrich must be true")
	}

	// A reader with no grant is denied even the read.
	if _, err := store.Get(none); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("no-grant read err = %v, want permission denied", err)
	}

	// A rep (read-only) cannot toggle it.
	if _, err := store.Update(rep, capturemod.SettingsPatch{AutoEnrich: integration.BoolPtr(false)}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("rep update err = %v, want permission denied", err)
	}
	if auditCount() != 0 {
		t.Fatal("a denied update must write no audit row")
	}

	// Admin turns it off — one audit row, the new value returned and readable.
	updated, err := store.Update(admin, capturemod.SettingsPatch{AutoEnrich: integration.BoolPtr(false)})
	if err != nil {
		t.Fatalf("admin update: %v", err)
	}
	if updated.AutoEnrich {
		t.Fatal("update to false must return auto_enrich=false")
	}
	if auditCount() != 1 {
		t.Fatalf("admin update wrote %d audit rows, want 1", auditCount())
	}
	if reread, err := store.Get(rep); err != nil || reread.AutoEnrich {
		t.Fatalf("re-read after update: %+v err=%v — want auto_enrich=false", reread, err)
	}

	// An idempotent update (same value) is a no-op: no second audit row.
	if _, err := store.Update(admin, capturemod.SettingsPatch{AutoEnrich: integration.BoolPtr(false)}); err != nil {
		t.Fatalf("idempotent update: %v", err)
	}
	if auditCount() != 1 {
		t.Fatalf("idempotent update wrote a spurious audit row (%d total)", auditCount())
	}

	// A nil patch leaves it unchanged and writes nothing.
	if _, err := store.Update(admin, capturemod.SettingsPatch{}); err != nil {
		t.Fatalf("empty patch: %v", err)
	}
	if auditCount() != 1 {
		t.Fatal("an empty patch must write no audit row")
	}
}

// The website-reading limits ride the same store and the same gate as the
// auto-enrich switch: every role reads them, only the update grant changes
// them, each change is audited, and one PATCH commits all of its fields or
// none of them.
func TestTheWebsiteReadingLimitsAreSettingsOnlyTheUpdateGrantChanges(t *testing.T) {
	e := integration.SetupSearch(t)
	store := capturemod.NewSettings(compose.NewSettingsStore(e.Pool))
	admin := captureSettingsCtx(e, principal.ObjectGrant{Read: true, Update: true})
	rep := captureSettingsCtx(e, principal.ObjectGrant{Read: true})

	got, err := store.Get(rep)
	if err != nil {
		t.Fatalf("rep read: %v", err)
	}
	defaults := capturemod.SiteReadLimits{
		MaxPages:    capturemod.DefaultSiteReadMaxPages,
		MaxMiB:      capturemod.DefaultSiteReadMaxMiB,
		WallSeconds: capturemod.DefaultSiteReadWallSeconds,
	}
	if got.AutoEnrichDailyCap != capturemod.DefaultAutoEnrichDailyCap || got.SiteRead != defaults {
		t.Fatalf("an untouched installation reads cap %d and limits %+v, want the declared defaults", got.AutoEnrichDailyCap, got.SiteRead)
	}

	dailyCap, pages := 2000, 20
	if _, err := store.Update(rep, capturemod.SettingsPatch{AutoEnrichDailyCap: &dailyCap}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("rep update err = %v, want permission denied", err)
	}

	audits := func() int {
		var n int
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(),
				`SELECT count(*) FROM audit_log WHERE entity_type = 'capture_settings'`).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := audits()
	updated, err := store.Update(admin, capturemod.SettingsPatch{AutoEnrichDailyCap: &dailyCap, SiteReadMaxPages: &pages})
	if err != nil {
		t.Fatalf("admin update: %v", err)
	}
	if updated.AutoEnrichDailyCap != dailyCap || updated.SiteRead.MaxPages != pages {
		t.Fatalf("update returned cap %d and pages %d, want %d and %d", updated.AutoEnrichDailyCap, updated.SiteRead.MaxPages, dailyCap, pages)
	}
	if n := audits() - before; n != 2 {
		t.Fatalf("changing two limits wrote %d audit rows, want one per setting", n)
	}

	// One field out of range refuses the whole patch, the valid field with it.
	tooLong, fewer := capturemod.MaxSiteReadWallSeconds+1, 5
	_, err = store.Update(admin, capturemod.SettingsPatch{SiteReadMaxPages: &fewer, SiteReadWallSeconds: &tooLong})
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) || invalid.Setting != capturemod.SiteReadWallSeconds.Key() {
		t.Fatalf("an out-of-range wall gave %v, want the wall's own refusal", err)
	}
	if reread, err := store.Get(rep); err != nil || reread.SiteRead.MaxPages != pages {
		t.Fatalf("after the refused patch the page limit reads %d (err %v), want the %d that stood", reread.SiteRead.MaxPages, err, pages)
	}
	if n := audits() - before; n != 2 {
		t.Fatalf("a refused patch wrote %d more audit rows, want none", n-2)
	}
}
