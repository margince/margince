// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The decisions a start makes before it spawns anything.
//
// Each is a pure function over a file or a string, and each answers a question
// a user's data depends on — which port to listen on, and whether a cluster
// that already exists is left alone. They ran nowhere: this module is outside
// go.work, so `./...` inside backend cannot reach them, and until the module
// got a lane of its own the tests it did carry ran nowhere either.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The port a user chose, and the refusal for one the OS would reject.
//
// A bad value has to fail HERE, naming the file and the key, or it surfaces
// later as a listen error about a number the user cannot connect to what they
// typed.
func TestResolvePortReadsTheSettingOrRefusesItByName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     []string
		want    int
		wantErr string
	}{
		{name: "no setting at all falls back to the default", want: defaultPort},
		{name: "an empty value is not a setting", env: []string{"MARGINCE_PORT="}, want: defaultPort},
		{name: "another key is left alone", env: []string{"MARGINCE_OTHER=9"}, want: defaultPort},
		{name: "a number is taken", env: []string{"MARGINCE_PORT=9123"}, want: 9123},
		{name: "the first setting wins", env: []string{"MARGINCE_PORT=9123", "MARGINCE_PORT=9124"}, want: 9123},
		{name: "the lowest port a listener can take", env: []string{"MARGINCE_PORT=1"}, want: 1},
		{name: "the highest", env: []string{"MARGINCE_PORT=65535"}, want: 65535},
		{name: "zero is not a port", env: []string{"MARGINCE_PORT=0"}, wantErr: `"0"`},
		{name: "one past the top", env: []string{"MARGINCE_PORT=65536"}, wantErr: `"65536"`},
		{name: "a negative number", env: []string{"MARGINCE_PORT=-1"}, wantErr: `"-1"`},
		{name: "words are not numbers", env: []string{"MARGINCE_PORT=eighty"}, wantErr: `"eighty"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port, err := resolvePort(tc.env)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("resolvePort(%v) = %d, want a refusal", tc.env, port)
				}
				// The value the user typed, quoted back: a refusal that does not
				// name it leaves them looking for a setting they cannot find.
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("the refusal reads %q, want it to quote %s", err, tc.wantErr)
				}
				if !strings.Contains(err.Error(), "margince.env") {
					t.Errorf("the refusal reads %q, want it to name the file to edit", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePort(%v): %v", tc.env, err)
			}
			if port != tc.want {
				t.Errorf("resolvePort(%v) = %d, want %d", tc.env, port, tc.want)
			}
		})
	}
}

// Whether a cluster already exists, which decides whether one is CREATED.
//
// The wrong answer in one direction re-runs initdb over somebody's database.
// PG_VERSION is the marker because initCluster renames a finished staging
// directory into place, so the file exists only for a cluster initdb completed.
func TestNeedsInitdbAnswersFromTheClusterMarker(t *testing.T) {
	l := newTestLayout(t)

	needs, err := needsInitdb(l)
	if err != nil {
		t.Fatal(err)
	}
	if !needs {
		t.Fatal("an installation with no data directory was read as already holding a cluster")
	}

	if err := os.MkdirAll(l.pgData(), 0o700); err != nil {
		t.Fatal(err)
	}
	// A directory alone is not a cluster: an interrupted creation leaves one,
	// and reading it as finished is how a half-built database gets started.
	if needs, err = needsInitdb(l); err != nil || !needs {
		t.Fatalf("an empty data directory: needs=%t err=%v, want true", needs, err)
	}

	if err := os.WriteFile(filepath.Join(l.pgData(), "PG_VERSION"), []byte("16\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if needs, err = needsInitdb(l); err != nil || needs {
		t.Fatalf("a finished cluster: needs=%t err=%v, want false — initdb would run over the "+
			"user's own database", needs, err)
	}
}

// A first run writes the settings template; no later run touches it.
//
// It carries secrets and the user's own choices, and an update that rewrote it
// would take both away — which is exactly what the template's own first lines
// promise it will not do.
func TestTheSettingsTemplateNeverReplacesAUsersOwnChoices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "margince.env")

	if err := ensureEnvFile(path); err != nil {
		t.Fatalf("first run: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("the settings file was created empty, so it documents nothing a user could turn on")
	}
	// Readable only by its owner: it is the file the template tells a user to
	// put a licence token and a mail credential into.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("the settings file is mode %04o, want 0600 — it holds secrets", perm)
	}

	edited := "MARGINCE_PORT=9123\n"
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureEnvFile(path); err != nil {
		t.Fatalf("second run: %v", err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != edited {
		t.Error("a later start rewrote the settings file, taking the user's own choices with it")
	}
}

// A socket path the system cannot take is refused BEFORE the database is
// started, with the move that fixes it.
//
// The limit is the kernel's, and the failure it produces if the launcher does
// not check is a bind error naming a byte count — a sentence that tells a user
// nothing about the folder they chose.
func TestADeeplyNestedInstallationIsRefusedWithSomethingToDo(t *testing.T) {
	shallow := layout{root: shortTempDir(t)}
	dir, err := resolveSocketDir(shallow)
	if err != nil {
		t.Fatalf("an ordinary installation was refused: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("the socket directory was not created: %v", err)
	}

	deep := layout{root: filepath.Join(shallow.root, strings.Repeat("a-long-folder-name/", 12))}
	_, err = resolveSocketDir(deep)
	if err == nil {
		t.Fatal("a socket path past the system limit was accepted; the failure moves to a bind " +
			"error about a byte count, which names nothing the user chose")
	}
	for _, want := range []string{"too deeply nested", "Move the Margince folder"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal reads %q, want it to contain %q", err, want)
		}
	}
}

// shortTempDir is a scratch directory under a SHORT base, because the ordinary
// one is not.
//
// The unix socket path limit this test is about is around a hundred bytes, and
// macOS hands `t.TempDir()` a path of its own that is most of that before the
// installation's own folders are added — so a fixture built there is refused
// for the reason the test means to prove is unusual.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mgl")
	if err != nil {
		t.Fatalf("a short scratch directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("removing %s: %v", dir, err)
		}
	})
	return dir
}
