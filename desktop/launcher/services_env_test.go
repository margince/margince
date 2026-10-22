// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// A desktop installation has local disk and no object storage service, so the
// launcher points the blobstore at a folder inside it. Without the default,
// attachments and company logos answer 501 until somebody edits margince.env.
func TestChildEnvDefaultsTheBlobstorePathIntoTheInstallation(t *testing.T) {
	l := layout{root: t.TempDir()}
	b := &backend{layout: l}

	got := valueOf(t, b.childEnv(), "MARGINCE_BLOBSTORE_PATH")

	want := filepath.Join(l.data(), "blobs")
	if got != want {
		t.Errorf("MARGINCE_BLOBSTORE_PATH = %q, want %q", got, want)
	}
}

// The default is a DEFAULT: an operator who names another store in
// margince.env keeps it. The roles read that file through --env-file, below the
// environment, so the launcher must not set the variable over it.
func TestChildEnvLeavesABlobstorePathMargineEnvNamesToTheFile(t *testing.T) {
	b := &backend{
		layout:  layout{root: t.TempDir()},
		userEnv: []string{"MARGINCE_BLOBSTORE_PATH=/somewhere/else"},
	}

	for _, entry := range b.childEnv() {
		if strings.HasPrefix(entry, "MARGINCE_BLOBSTORE_PATH=") {
			t.Errorf("childEnv sets %q over the value in margince.env", entry)
		}
	}
}

// The user's settings reach the roles through the file alone, so a secret in
// margince.env stays out of a child's environment.
func TestChildEnvCarriesNoUserSetting(t *testing.T) {
	b := &backend{
		layout:  layout{root: t.TempDir()},
		userEnv: []string{"GEMINI_API_KEY=from-the-file"},
	}

	for _, entry := range b.childEnv("MARGINCE_DSN=owned") {
		if strings.HasPrefix(entry, "GEMINI_API_KEY=") {
			t.Errorf("childEnv carries %q, which the roles read from margince.env", entry)
		}
	}
	if got := valueOf(t, b.childEnv("MARGINCE_DSN=owned"), "MARGINCE_DSN"); got != "owned" {
		t.Errorf("MARGINCE_DSN = %q, want the launcher's own", got)
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
