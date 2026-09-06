// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The sign-in policy read apart from the installation settings around it.
//
// `GET /installation/settings` carries the installation's name, timezone and
// base currency, which every role reads — a rep reading amounts benefits from
// knowing the currency. It also carried `sign_in_providers`, which is
// authentication administration and has no business being handed to every seat
// holding `installation_settings.read`.
//
// The obvious fix does not work and this file is built around why. The values
// come from a settings ENTRY, and the settings catalog gates both verbs on an
// entry's single object, so repointing EnabledOidcProviders at
// authentication_policy would make every read of the aggregate demand the new
// grant — the name, the timezone and the currency with it. So the projection
// carries the gate and reads the entry as the installation afterwards.
//
// That shape puts the whole security of the read in one `auth.Require`: the
// system principal underneath it bypasses object RBAC entirely. What follows
// therefore proves three things, and the third is the one a unit test cannot
// reach:
//
//   the grant admits, and its absence refuses
//   the ordinary installation read still works WITHOUT the new grant
//   installation_settings.read alone does NOT open the sign-in policy

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// authPolicyCtx builds a caller holding exactly the grants named, so a case can
// state which object it is testing and hold every other one absent. The
// one-object helper beside it cannot express "installation_settings but not
// authentication_policy", which is the pair this split turns on.
func (e *SearchEnv) authPolicyCtx(grants map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}

func TestSignInPolicyAnswersItsOwnGrantAndRefusesWithoutIt(t *testing.T) {
	e := SetupSearch(t)
	store := identity.NewInstallationSettings(e.DB(), compose.NewSettingsStore(e.Pool))

	// Admitted on the new grant alone: the projection does not also require the
	// aggregate's object, or a reader who may administer sign-in but not rename
	// the company would be refused.
	holder := e.authPolicyCtx(map[string]principal.ObjectGrant{
		"authentication_policy": {Read: true},
	})
	if _, err := store.SignInPolicy(holder); err != nil {
		t.Fatalf("an authentication_policy holder was refused the sign-in policy: %v", err)
	}

	// Refused without it. This is the assertion the system principal underneath
	// makes load-bearing: with the Require gone, this caller reads the entry as
	// the installation and the refusal disappears silently.
	none := e.authPolicyCtx(map[string]principal.ObjectGrant{})
	if _, err := store.SignInPolicy(none); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("read without a grant returned %v, want ErrPermissionDenied", err)
	}
}

// The reason the projection exists rather than a moved entry: the ordinary
// installation read must keep working for a reader who holds nothing else.
//
// If this ever fails, the sign-in split has taken the company name and the
// reporting currency with it, and every role that reads amounts is looking at a
// permission error instead.
func TestTheInstallationReadSurvivesWithoutTheSignInGrant(t *testing.T) {
	e := SetupSearch(t)
	store := identity.NewInstallationSettings(e.DB(), compose.NewSettingsStore(e.Pool))

	rep := e.authPolicyCtx(map[string]principal.ObjectGrant{
		"installation_settings": {Read: true},
	})
	got, err := store.GetInstallation(rep)
	if err != nil {
		t.Fatalf("the installation read now demands the sign-in grant: %v", err)
	}
	if got.Timezone != "UTC" || got.BaseCurrency != "EUR" {
		t.Errorf("unset settings read as timezone=%q currency=%q, want the registered defaults UTC/EUR",
			got.Timezone, got.BaseCurrency)
	}
}

// And the other direction, which is the whole point of splitting: holding the
// aggregate's grant does not carry the sign-in policy with it.
//
// Without this case the split could be entirely undone — by gating the
// projection on installation_settings — and every other test here would still
// pass.
func TestTheInstallationGrantDoesNotOpenTheSignInPolicy(t *testing.T) {
	e := SetupSearch(t)
	store := identity.NewInstallationSettings(e.DB(), compose.NewSettingsStore(e.Pool))

	rep := e.authPolicyCtx(map[string]principal.ObjectGrant{
		"installation_settings": {Read: true, Update: true},
	})
	if _, err := store.SignInPolicy(rep); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("installation_settings alone opened the sign-in policy: %v", err)
	}
}

// The split is only HALF done, and this records which half.
//
// A narrow endpoint beside an aggregate that still hands the same field to every
// role is a second door onto data nobody closed: `GET /installation/settings`
// continues to return `sign_in_providers` on `installation_settings.read`, so
// today the new grant buys a tidier surface and no confidentiality.
//
// Closing it means REMOVING a published response field, which
// `contract-breaking-check` refuses — correctly, since a client reading that
// field would break. The supported route is to deprecate it, ship a release
// where both exist, and remove it once clients have moved. That is a product
// decision about a published contract, not something to slip into this change.
//
// This test is written to FAIL when the leak is closed, so the day somebody
// deprecates and removes the field, the suite says so rather than staying quiet.
// Until then it asserts the leak is still exactly where it is: no narrower, so
// nobody believes the split is finished, and no wider.
func TestTheAggregateStillCarriesTheSignInProvidersUntilItIsDeprecated(t *testing.T) {
	e := SetupSearch(t)
	store := identity.NewInstallationSettings(e.DB(), compose.NewSettingsStore(e.Pool))

	// A provider must actually be STORED first. With none stored the field is nil
	// whatever the aggregate does, and this test would pass against a surface
	// that discloses everything — the vacuous green a leak test must not have.
	writer := e.authPolicyCtx(map[string]principal.ObjectGrant{
		"installation_settings": {Read: true, Update: true},
	})
	chosen := []string{"google"}
	if _, err := store.UpdateInstallation(writer, identity.InstallationPatch{
		EnabledOidcProviders: &chosen,
	}); err != nil {
		t.Fatalf("storing a sign-in provider: %v", err)
	}

	rep := e.authPolicyCtx(map[string]principal.ObjectGrant{
		"installation_settings": {Read: true},
	})
	got, err := store.GetInstallation(rep)
	if err != nil {
		t.Fatalf("the installation read failed: %v", err)
	}
	if len(got.EnabledOidcProviders) == 0 {
		t.Fatal("the aggregate no longer carries the sign-in providers — if the field " +
			"was deprecated and removed on purpose, delete this test and the comment " +
			"above it; the split is then complete")
	}
}

// The WRITE half is unsplit too, and for the same reason: `enabled_oidc_providers`
// is a published field of `PATCH /installation/settings`, gated on
// `installation_settings.update` through the settings entry.
//
// So an ops seat — or any custom role holding installation-settings update
// without authentication-policy update — can still turn a sign-in provider on or
// off. The read endpoint above narrows who SEES the policy; nothing yet narrows
// who CHANGES it, and a reader of that endpoint's description should not have to
// infer that.
//
// Like its read counterpart this is written to FAIL when the gap closes, so the
// change that finally gates the write is told to come back and delete it.
func TestTheSettingsPatchStillChangesSignInProvidersUntilItIsSplit(t *testing.T) {
	e := SetupSearch(t)
	store := identity.NewInstallationSettings(e.DB(), compose.NewSettingsStore(e.Pool))

	// Deliberately WITHOUT authentication_policy: this is the caller the split is
	// supposed to be keeping out of the sign-in policy.
	opsish := e.authPolicyCtx(map[string]principal.ObjectGrant{
		"installation_settings": {Read: true, Update: true},
	})
	chosen := []string{"google"}
	if _, err := store.UpdateInstallation(opsish, identity.InstallationPatch{
		EnabledOidcProviders: &chosen,
	}); err != nil {
		t.Fatalf("installation_settings.update can no longer change the sign-in providers "+
			"(%v) — if the write was split onto authentication_policy on purpose, delete "+
			"this test and the note in the endpoint description; the split is then complete", err)
	}
}
