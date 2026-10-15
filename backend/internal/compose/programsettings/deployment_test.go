// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package programsettings_test

import (
	"strings"
	"testing"

	ps "github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// An installation with no auth section signs in by email and password.
func TestPasswordLoginIsOnUnlessTheFileTurnsItOff(t *testing.T) {
	t.Parallel()
	if !config.Val(configtest.MustLoad(t, "version: 1\n", ps.FileSettings()...), ps.PasswordLogin) {
		t.Fatal("a file with no auth section turned password sign-in off")
	}
	// The file cannot check that something else opens a door; cmd/api refuses
	// the combination that leaves no way in.
	off := configtest.MustLoad(t, "version: 1\nauth: { password: { enabled: false } }\n", ps.FileSettings()...)
	if config.Val(off, ps.PasswordLogin) {
		t.Fatal("auth.password.enabled=false read as enabled; the switch would be inert")
	}
}

// A typo must never silently disable authentication.
func TestATypoUnderAuthStopsTheBoot(t *testing.T) {
	t.Parallel()
	_, err := configtest.Load(t, "version: 1\nauth:\n  passwrd:\n    enabled: false\n", nil, nil, ps.FileSettings()...)
	if err == nil || !strings.Contains(err.Error(), "passwrd") {
		t.Fatalf("an unknown key loaded silently: %v", err)
	}
}

// Capture stores content close to special categories, so it is off unless the
// file says so.
func TestAIPayloadCaptureIsOffUnlessTheFileTurnsItOn(t *testing.T) {
	t.Parallel()
	if config.Val(configtest.MustLoad(t, "version: 1\n", ps.FileSettings()...), ps.CapturePayloads) {
		t.Fatal("ai.capture_payloads must default to false")
	}
	if !config.Val(configtest.MustLoad(t, "version: 1\nai:\n  capture_payloads: true\n", ps.FileSettings()...), ps.CapturePayloads) {
		t.Fatal("ai.capture_payloads: true read as off")
	}
}
