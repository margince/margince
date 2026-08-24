// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// A desktop installation has local disk and no object storage service, so the
// launcher points the blobstore at one inside the folder. Without the default the
// api wires no store and attachments and company logos answer 501 until somebody
// edits margince.env — which they CAN do, since childEnv passes the user's own
// values through; what the default buys is that nobody has to. A feature that
// needs a settings file opened before it works is a feature most of this
// bundle's users never find.
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
