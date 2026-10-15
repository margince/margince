// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// layered writes a base and (when non-empty) an overlay for the posture, and
// loads them the way a boot does.
func layered(t *testing.T, env runtimeenv.Environment, base, overlay string) (Config, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "margince.yaml")
	if err := os.WriteFile(path, []byte(base), 0o600); err != nil {
		t.Fatalf("writing the base: %v", err)
	}
	if overlay != "" {
		if err := os.WriteFile(OverlayPath(path, env), []byte(overlay), 0o600); err != nil {
			t.Fatalf("writing the overlay: %v", err)
		}
	}
	return Load(path, env)
}

func TestTheOverlayForThisPostureWinsKeyByKey(t *testing.T) {
	cfg, err := layered(t, runtimeenv.Development,
		"version: 1\nworkspace:\n  name: Acme\n  timezone: Europe/Berlin\nseeds:\n  booking_page: false\n",
		"seeds:\n  booking_page: true\n")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if bp := cfg.Seeds.BookingPage; bp == nil || !*bp {
		t.Error("the overlay set seeds.booking_page: true and the base's false survived")
	}
	// The keys the overlay is silent about are the reason for having a base at
	// all: an overlay states a difference, not a whole configuration.
	if cfg.Workspace.Name != "Acme" || cfg.Workspace.Timezone != "Europe/Berlin" {
		t.Errorf("the base's workspace did not survive the overlay: %+v", cfg.Workspace)
	}
}

// A posture that arms nothing must not inherit another posture's arming — the
// file is selected by name, so only the running posture's overlay is read.
func TestOnlyTheRunningPosturesOverlayIsRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "margince.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("writing the base: %v", err)
	}
	if err := os.WriteFile(OverlayPath(path, runtimeenv.Development),
		[]byte("seeds:\n  booking_page: false\n"), 0o600); err != nil {
		t.Fatalf("writing the dev overlay: %v", err)
	}
	prod, err := Load(path, runtimeenv.Production)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prod.Seeds.BookingPage != nil {
		t.Fatal("production read the dev overlay")
	}
	dev, err := Load(path, runtimeenv.Development)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if dev.Seeds.BookingPage == nil {
		t.Error("the dev overlay was not read for the posture it names")
	}
}

// An overlay committed as a placeholder — the posture exists, its keys are
// still commented out — is a file a boot must read and shrug at.
func TestACommentOnlyOverlayIsNotAFailure(t *testing.T) {
	cfg, err := layered(t, runtimeenv.Development,
		"version: 1\nworkspace:\n  name: Acme\n",
		"# nothing to change for dev yet\n")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Workspace.Name != "Acme" {
		t.Errorf("workspace.name = %q, want Acme", cfg.Workspace.Name)
	}
}

// Strictness is per layer, not just for the base: an overlay is where a typo is
// LIKELIER, because it is the file edited per posture and the one with no
// example to copy from.
func TestATypoInTheOverlayFailsTheBootAndNamesTheFile(t *testing.T) {
	_, err := layered(t, runtimeenv.Development,
		"version: 1\n",
		"workspace:\n  nme: Acme\n")
	if err == nil {
		t.Fatal("a misspelled key in the overlay booted; a typo must never silently do nothing")
	}
	if !strings.Contains(err.Error(), "margince.dev.yaml") {
		t.Errorf("error = %q; it must name which of the two files holds the typo", err)
	}
}

// Validation runs over the MERGED result, never per file. An overlay that
// completes what the base only starts is the ordinary way a posture supplies
// its own admin, and validating the base alone would refuse it.
func TestValidationRunsOverTheMergedResult(t *testing.T) {
	cfg, err := layered(t, runtimeenv.Test,
		"version: 1\nworkspace:\n  name: Acme\nbootstrap_admin:\n  email: admin@acme.test\n  display_name: Admin\n  password_file: /run/secrets/base-admin\n",
		"bootstrap_admin:\n  password_file: /run/secrets/test-admin\n")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BootstrapAdmin == nil {
		t.Fatal("bootstrap_admin vanished across the merge")
	}
	if cfg.BootstrapAdmin.Email != "admin@acme.test" {
		t.Errorf("email = %q; the base's value survived an overlay that only replaced the password reference", cfg.BootstrapAdmin.Email)
	}
}

func TestOverlayPathInsertsThePostureBeforeTheExtension(t *testing.T) {
	for _, tc := range []struct {
		base string
		env  runtimeenv.Environment
		want string
	}{
		{"config/margince.yaml", runtimeenv.Development, "config/margince.dev.yaml"},
		{"config/margince.yaml", runtimeenv.Test, "config/margince.test.yaml"},
		{"config/margince.yaml", runtimeenv.Production, "config/margince.production.yaml"},
		{"/etc/margince/deploy.yml", runtimeenv.Production, "/etc/margince/deploy.production.yml"},
	} {
		if got := OverlayPath(tc.base, tc.env); got != tc.want {
			t.Errorf("OverlayPath(%q, %q) = %q, want %q", tc.base, tc.env, got, tc.want)
		}
	}
}

// Neither file is required. This is how an already-bootstrapped installation
// runs: the ADR permits deleting the file once the company exists.
func TestNeitherLayerHasToExist(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.yaml"), runtimeenv.Development)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d; the compiled default is the first layer", cfg.Version)
	}
}
