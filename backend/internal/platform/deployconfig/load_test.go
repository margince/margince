// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// config.Load skips these roots and no others.
func TestTheRootsAreTheFourBootstrapKeys(t *testing.T) {
	got := slices.Sorted(maps.Keys(Roots()))
	if want := []string{"bootstrap_admin", "seeds", "version", "workspace"}; !slices.Equal(got, want) {
		t.Fatalf("Roots() = %v, want %v", got, want)
	}
}

// Every other root belongs to config.Load, so this package reads past it.
func TestTheSettingsRootsAreLeftToTheSettingsLoader(t *testing.T) {
	boot, err := LoadBootstrap(writeTemp(t, "version: 1\nworkspace:\n  name: Acme\nauth:\n  password:\n    enabled: false\nrates:\n  fx_currencies: [SEK]\n"))
	if err != nil || boot.Workspace.Name != "Acme" {
		t.Fatalf("boot %+v, err %v; want the workspace read and the settings roots skipped", boot, err)
	}
}

// A root the --config-file writes replaces the base's whole, and the roots it
// leaves out keep the base's values.
func TestTheConfigFileReplacesTheRootsItWrites(t *testing.T) {
	base := writeTemp(t, "version: 1\nworkspace:\n  name: Acme\n  timezone: Europe/Berlin\nseeds:\n  booking_page: false\n  legal_review: false\n")
	over := writeTemp(t, "seeds:\n  booking_page: true\n")
	boot, err := LoadBootstrapFiles(base, over)
	if err != nil {
		t.Fatalf("LoadBootstrapFiles: %v", err)
	}
	if bp := boot.Seeds.BookingPage; bp == nil || !*bp {
		t.Error("the config file set seeds.booking_page: true and the base's false survived")
	}
	if boot.Seeds.LegalReview != nil {
		t.Error("seeds.legal_review survived from the base; the config file's seeds replace the base's whole")
	}
	if boot.Workspace.Name != "Acme" || boot.Workspace.Timezone != "Europe/Berlin" {
		t.Errorf("the base's workspace did not survive: %+v", boot.Workspace)
	}
}

// A typo is a boot error in either file, and names the file that holds it.
func TestATypoInTheConfigFileFailsAndNamesTheFile(t *testing.T) {
	over := writeTemp(t, "workspace:\n  nme: Acme\n")
	_, err := LoadBootstrapFiles(writeTemp(t, "version: 1\n"), over)
	if err == nil || !strings.Contains(err.Error(), over) {
		t.Fatalf("err %v; want a refusal naming %s", err, over)
	}
}

// Validation runs over the result, never per file: a config file that
// supplies a whole bootstrap_admin completes a base that only started one.
func TestValidationRunsOverTheReplacedResult(t *testing.T) {
	base := writeTemp(t, "version: 1\nbootstrap_admin:\n  email: base@acme.test\n  display_name: Admin\n")
	over := writeTemp(t, "bootstrap_admin:\n  email: admin@acme.test\n  display_name: Admin\n  password_file: /run/secrets/test-admin\n")
	boot, err := LoadBootstrapFiles(base, over)
	if err != nil || boot.BootstrapAdmin == nil || boot.BootstrapAdmin.Email != "admin@acme.test" {
		t.Fatalf("boot %+v, err %v", boot.BootstrapAdmin, err)
	}
}

// A missing file, or one holding only comments, is an installation with
// nothing to bootstrap.
func TestNeitherFileHasToHoldAnything(t *testing.T) {
	for _, files := range [][2]string{
		{filepath.Join(t.TempDir(), "absent.yaml"), ""},
		{writeTemp(t, "# all of it commented out\n"), writeTemp(t, "# a placeholder\n")},
	} {
		boot, err := LoadBootstrapFiles(files[0], files[1])
		if err != nil || boot.Version != 1 || boot.BootstrapAdmin != nil {
			t.Errorf("%v: boot %+v, err %v; want version 1 and nothing to bootstrap", files, boot, err)
		}
	}
}

// The dev stack reads the tracked dev file as its --config-file, and its
// seeds bind the dev models when a fresh database is bootstrapped.
func TestTheShippedDevFileSeedsTheDevBinding(t *testing.T) {
	boot, err := LoadBootstrapFiles("../../../../config/margince.example.yaml", "../../../../config/margince.dev.yaml")
	if err != nil {
		t.Fatalf("config/margince.dev.yaml no longer loads over the example: %v", err)
	}
	if boot.Seeds.AIRouting.IsZero() {
		t.Error("the dev file's seeds.ai_routing did not reach the bootstrap")
	}
}
