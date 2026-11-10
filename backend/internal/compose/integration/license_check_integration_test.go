// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// "Check now" on Settings → Seats: POST /installation/license/check
// runs the api's license re-check at once. These tests drive a real
// licensecheck.Watcher over a token source the test swaps, through the HTTP
// route, so the gate, the re-read and the answer are the ones production runs.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

const licenseCheckPath = "/v1/installation/license/check"

// swappableToken is the deployment's token as the operator leaves it on disk:
// the test changes it the way an operator replaces the file.
type swappableToken struct {
	token string
	err   error
	reads int
}

func (s *swappableToken) source() (string, error) {
	s.reads++
	return s.token, s.err
}

// licenseCheckApp boots the api with a real watcher over source, wired the way
// cmd/api wires it: the posture for the reads, the re-check for "Check now".
func licenseCheckApp(t *testing.T, source licensecheck.TokenSource) *apptest.AppEnv {
	t.Helper()
	clock := func() time.Time { return clocktest.Now(t) }
	watcher, err := licensecheck.NewWatcher(context.Background(), source, clock,
		slog.New(slog.DiscardHandler), runtimeenv.Test)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	e := apptest.SetupAppWithOptions(t,
		compose.WithLicensePosture(watcher.Posture),
		compose.WithLicenseRecheck(watcher.Recheck))
	e.BootstrapWorkspace(t)
	return e
}

// The swapped token applies on the press, not a day later. No token this
// repository can mint is honored by the bundled keyset, so the swap goes from
// no token to one the module refuses: the posture still has to move from
// absent to rejected, which only a fresh read of the source can do.
func TestCheckNowAppliesASwappedToken(t *testing.T) {
	source := &swappableToken{}
	e := licenseCheckApp(t, source.source)

	var before entitlement
	if status := e.Call(t, "GET", "/v1/installation/license", nil, nil, &before); status != http.StatusOK {
		t.Fatalf("read the entitlement → %d", status)
	}
	if before.State != "absent" {
		t.Fatalf("state before the swap = %q, want absent", before.State)
	}

	source.token = "a-replaced-token-the-module-refuses"
	var after entitlement
	if status := e.Call(t, "POST", licenseCheckPath, nil, nil, &after); status != http.StatusOK {
		t.Fatalf("Check now → %d, want 200 (a refusal is a verdict, not a fault)", status)
	}
	if after.State != "rejected" {
		t.Errorf("state after Check now = %q, want rejected — the replaced token was not re-read", after.State)
	}
	if after.SeatsUsed != before.SeatsUsed {
		t.Errorf("seats used = %d after Check now, want the %d the GET reported", after.SeatsUsed, before.SeatsUsed)
	}

	// The GET reports the same posture afterwards: the press updated the
	// watcher, not just its own answer.
	var reread entitlement
	if status := e.Call(t, "GET", "/v1/installation/license", nil, nil, &reread); status != http.StatusOK {
		t.Fatalf("read the entitlement after Check now → %d", status)
	}
	if reread.State != "rejected" {
		t.Errorf("GET after Check now = %q, want rejected", reread.State)
	}
}

// A token that cannot be read is no verdict. The answer is 503, and the
// posture the process held stands.
func TestCheckNowReportsATokenItCouldNotRead(t *testing.T) {
	source := &swappableToken{}
	e := licenseCheckApp(t, source.source)

	source.err = errors.New("license.token_file: no such file")
	if status := e.Call(t, "POST", licenseCheckPath, nil, nil, nil); status != http.StatusServiceUnavailable {
		t.Fatalf("Check now on an unreadable token → %d, want 503", status)
	}
	var got entitlement
	if status := e.Call(t, "GET", "/v1/installation/license", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("read the entitlement → %d", status)
	}
	if got.State != "absent" {
		t.Errorf("state after a failed re-read = %q, want it to stand at absent", got.State)
	}
}

// A seat without installation_settings update is refused, and the api does not
// re-read its token on that seat's behalf.
func TestCheckNowRefusesANonAdmin(t *testing.T) {
	source := &swappableToken{}
	e := licenseCheckApp(t, source.source)
	e.DescribeCompany(t)

	var invited struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/users", map[string]any{
		"email": "rep@license-check.test", "display_name": "A Rep", "role": "rep",
	}, nil, &invited); status != http.StatusCreated {
		t.Fatalf("invite a rep → %d, want 201", status)
	}
	const password = "Test-license-check-rep-password-23!"
	apptest.SetPasswordByLink(t, e, invited.ID, password)
	rep := e.AsSecondSeat(t, "rep@license-check.test", password)

	readsBefore := source.reads
	if status := rep.Call(t, "POST", licenseCheckPath, nil, nil, nil); status != http.StatusForbidden {
		t.Fatalf("a rep pressing Check now → %d, want 403", status)
	}
	if source.reads != readsBefore {
		t.Errorf("the token was read %d times for a refused caller, want 0", source.reads-readsBefore)
	}
}

// A token replaced in the file the deployment names is seen by "Check now",
// for both ways margince.yaml can name that file. The source is the one
// EnsureLicense builds from the loaded configuration. Only the boot read is
// answered as unlicensed: no token this repository can mint boots, and the
// watcher refuses a rejected one at boot.
func TestCheckNowReadsATokenReplacedInTheDeclaredFile(t *testing.T) {
	for name, declare := range map[string]func(path string) string{
		"license.token as a file reference": func(path string) string {
			return "license:\n  token: ${file:" + path + "}\n"
		},
		"license.token_file": func(path string) string {
			return "license:\n  token_file: " + path + "\n"
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "license")
			writeToken(t, path, "the-token-at-boot")
			cfg, err := configtest.Load(t, declare(path), nil, nil, compose.LicenseToken, compose.LicenseTokenFile)
			if err != nil {
				t.Fatalf("loading the licence settings: %v", err)
			}
			declared := compose.SealedLicenseTokenSource(context.Background(), nil, nil, cfg, slog.New(slog.DiscardHandler))
			booted := false
			e := licenseCheckApp(t, func() (string, error) {
				if !booted {
					booted = true
					return "", nil
				}
				return declared()
			})

			writeToken(t, path, "a-replaced-token-the-module-refuses")
			var after entitlement
			if status := e.Call(t, "POST", licenseCheckPath, nil, nil, &after); status != http.StatusOK {
				t.Fatalf("Check now → %d, want 200", status)
			}
			if after.State != "rejected" {
				t.Errorf("state after Check now = %q, want rejected — the replaced file was not read", after.State)
			}
			got, err := declared()
			if err != nil || got != "a-replaced-token-the-module-refuses" {
				t.Errorf("the declared source answers %q, %v; want the replaced token", got, err)
			}
		})
	}
}

func writeToken(t *testing.T, path, token string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("writing the token file: %v", err)
	}
}
