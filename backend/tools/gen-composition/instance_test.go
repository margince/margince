// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testCoreVersion = "v0.0.0-20261104090510-75dae04dec39"

func TestRequiredCoreVersionsReadsEachCoreModule(t *testing.T) {
	t.Parallel()
	goMod := writeInstanceGoMod(t, "tool github.com/margince/margince/backend/tools/gen-composition\n\n"+
		"require (\n"+
		"\tgithub.com/margince/margince "+testCoreVersion+"\n"+
		"\tgithub.com/margince/margince/backend "+testCoreVersion+"\n"+
		"\tgithub.com/margince/margince/backend/tools "+testCoreVersion+" // indirect\n"+
		"\tgithub.com/acme/crm v1.2.0\n)\n")
	got, err := requiredCoreVersions(goMod)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range requiredCoreModules {
		if got[m.path] != testCoreVersion {
			t.Errorf("%s = %q, want %q", m.path, got[m.path], testCoreVersion)
		}
	}
}

func TestRequiredCoreVersionsNamesWhatIsMissing(t *testing.T) {
	t.Parallel()
	goMod := writeInstanceGoMod(t, "require github.com/margince/margince/backend "+testCoreVersion+"\n")
	_, err := requiredCoreVersions(goMod)
	if err == nil || !strings.Contains(err.Error(), coreRootModule+", "+coreToolsModule) {
		t.Fatalf("err = %v, want one naming the web app and the tools modules", err)
	}
}

func TestOneCoreCommitRefusesModulesFromDifferentCommits(t *testing.T) {
	t.Parallel()
	required := map[string]string{
		coreRootModule:  testCoreVersion,
		backendModule:   testCoreVersion,
		coreToolsModule: "v0.0.0-20261001000000-aaaaaaaaaaaa",
	}
	var downloaded []downloadedModule
	for path := range required {
		downloaded = append(downloaded, downloadedModule{Path: path, Dir: "/cache/" + path})
	}
	if _, _, err := oneCoreCommit(downloaded, required); err == nil || !strings.Contains(err.Error(), "different commits") {
		t.Fatalf("err = %v, want a refusal of mixed commits", err)
	}
	required[coreToolsModule] = testCoreVersion
	dirs, commit, err := oneCoreCommit(downloaded, required)
	if err != nil {
		t.Fatal(err)
	}
	if commit != "75dae04dec39" || dirs[backendModule] != "/cache/"+backendModule {
		t.Fatalf("commit = %q, dirs = %v", commit, dirs)
	}
}

func TestSameCommitMatchesAFullHashWithAPseudoVersionsRevision(t *testing.T) {
	t.Parallel()
	if !sameCommit("75dae04dec390f6263fcc18e4db95988da1763c5", "75dae04dec39") {
		t.Error("a full hash and its 12-character prefix are one commit")
	}
	if sameCommit("75dae04dec39", "75dae04dec30") || sameCommit("75dae", "75dae") {
		t.Error("different or too-short revisions must not match")
	}
}

func writeInstanceGoMod(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(path, []byte("module example.com/acme\n\ngo 1.27.4\n\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestATreeThatWouldHoldTheInstanceIsRefused(t *testing.T) {
	t.Parallel()
	instance := filepath.FromSlash("/work/acme")
	for _, tree := range []string{"/work/acme", "/work", "/"} {
		if err := checkTreeDir(instance, filepath.FromSlash(tree)); err == nil {
			t.Errorf("-tree %s was accepted; composing would delete the instance", tree)
		}
	}
	for _, tree := range []string{"/work/acme/build/core", "/work/acme/.margince/core", "/work/core", "/tmp/core"} {
		if err := checkTreeDir(instance, filepath.FromSlash(tree)); err != nil {
			t.Errorf("-tree %s was refused: %v", tree, err)
		}
	}
}

func TestCoreModulesAreNeverExtensions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"instance/go.mod": "module i\n\nrequire (\n" +
			"\tgithub.com/margince/margince " + testCoreVersion + "\n" +
			"\tgithub.com/margince/margince/backend " + testCoreVersion + "\n" +
			"\tgithub.com/margince/margince/backend/tools " + testCoreVersion + "\n" +
			"\texample.test/notes v0.0.0\n)\n\nreplace example.test/notes => ./notes\n",
	})
	units, err := instanceUnits(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].name != "notes" {
		t.Fatalf("units = %+v, want only notes: core's own modules required directly are not extensions", units)
	}
}
