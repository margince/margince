// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRegionalFormatsPersistAndRequireSettingsAuthority(t *testing.T) {
	e := SetupSearch(t)
	store := identity.NewInstallationSettings(e.DB(), compose.NewSettingsStore(e.Pool))
	admin := e.installationSettingsCtx(principal.ObjectGrant{Read: true, Update: true})
	rep := e.installationSettingsCtx(principal.ObjectGrant{Read: true})
	dateFormat, timeFormat := "dmy", "24h"
	patch := identity.InstallationPatch{DateFormat: &dateFormat, TimeFormat: &timeFormat}
	if _, err := store.UpdateInstallation(rep, patch); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("reader write: %v", err)
	}
	if _, err := store.UpdateInstallation(admin, patch); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetInstallation(rep)
	if err != nil {
		t.Fatal(err)
	}
	if got.DateFormat != dateFormat || got.TimeFormat != timeFormat {
		t.Fatalf("regional settings lost: %+v", got)
	}
	invalid := "guess"
	if _, err := store.UpdateInstallation(admin, identity.InstallationPatch{DateFormat: &invalid}); err == nil {
		t.Fatal("unsupported date format accepted")
	}
	if _, err := store.UpdateInstallation(admin, identity.InstallationPatch{TimeFormat: &invalid}); err == nil {
		t.Fatal("unsupported time format accepted")
	}
}
