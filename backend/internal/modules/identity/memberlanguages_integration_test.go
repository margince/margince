// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"slices"
	"testing"
)

// The settings say which languages members read the app in, and stop counting
// a member once they are gone.
func TestTheSettingsNameTheLanguagesActiveMembersChose(t *testing.T) {
	e := setupRevocationEnv(t, "member-languages")
	if _, err := e.svc.SaveMyLocale(e.wsCtx(e.member), "vi"); err != nil {
		t.Fatalf("the member choosing Vietnamese: %v", err)
	}
	languages := func() []string {
		t.Helper()
		got, err := e.allowanceSettings().GetInstallation(e.wsCtx(e.admin))
		if err != nil {
			t.Fatalf("reading the settings: %v", err)
		}
		return got.MemberLanguages
	}
	if got := languages(); !slices.Contains(got, "vi") {
		t.Fatalf("member languages = %v, want vi among them", got)
	}
	if err := e.svc.DeactivateUser(e.wsCtx(e.admin), e.admin, DeactivateUserInput{UserID: e.member.UserID}); err != nil {
		t.Fatalf("deactivating the member: %v", err)
	}
	if got := languages(); slices.Contains(got, "vi") {
		t.Fatalf("member languages = %v after the only Vietnamese reader left, want vi gone", got)
	}
}
