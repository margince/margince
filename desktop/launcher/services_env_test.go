// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// A desktop installation has local disk and no object storage service, so the
// launcher points the blobstore at one inside the folder. Without it attachments
// and company logos answer 501 — the api wires no store — and there is nothing a
// user can put in margince.env to fix that without running a separate service.
func TestChildEnvDefaultsTheBlobstorePathIntoTheInstallation(t *testing.T) {
	l := layout{root: t.TempDir()}
	b := &backend{layout: l}

	got := valueOf(t, b.childEnv(), "MARGINCE_BLOBSTORE_PATH")

	want := filepath.Join(l.data(), "blobs")
	if got != want {
		t.Errorf("MARGINCE_BLOBSTORE_PATH = %q, want %q", got, want)
	}
}

// The default is a DEFAULT: it sits in childEnv's first layer, so an operator
// who names a real object store in margince.env is not overridden by the folder
// the launcher would otherwise have used.
func TestChildEnvLetsMargineEnvOverrideTheBlobstorePath(t *testing.T) {
	b := &backend{
		layout:  layout{root: t.TempDir()},
		userEnv: []string{"MARGINCE_BLOBSTORE_PATH=/somewhere/else"},
	}

	if got := valueOf(t, b.childEnv(), "MARGINCE_BLOBSTORE_PATH"); got != "/somewhere/else" {
		t.Errorf("MARGINCE_BLOBSTORE_PATH = %q, want the value from margince.env", got)
	}
}

// valueOf returns the winning value of key in an environment slice — the LAST
// occurrence, which is the one exec resolves to.
func valueOf(t *testing.T, env []string, key string) string {
	t.Helper()
	value := ""
	found := false
	for _, entry := range env {
		if name, v, ok := strings.Cut(entry, "="); ok && name == key {
			value, found = v, true
		}
	}
	if !found {
		t.Fatalf("%s is not in the child environment: %v", key, env)
	}
	return value
}
