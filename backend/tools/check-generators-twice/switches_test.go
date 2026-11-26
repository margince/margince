// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"reflect"
	"testing"
)

const goldenFlagFile = `package pages

import (
	"flag"
	"os"
	"testing"
)

var updatePages = flag.Bool("update-pages", false, "rewrite the pages")

var verbose = flag.Bool("verbose-pages", false, "not a switch")

func TestThePagesAreCurrent(t *testing.T) { holdPage(t, "a.md") }

func TestTheSnapshotsAreCurrent(t *testing.T) {
	if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		return
	}
}

func TestSomethingElse(t *testing.T) { _ = *verbose }

func holdPage(t *testing.T, path string) {
	if *updatePages {
		return
	}
}
`

const taggedFile = `//go:build integration && sweep

package pages

import (
	"os"
	"testing"
)

func TestTheSweepIsPublished(t *testing.T) { holdSweep(t) }
`

const taggedHelperFile = `//go:build integration

package pages

import (
	"os"
	"testing"
)

func holdSweep(t *testing.T) {
	if os.Getenv("UPDATE_SWEEP") != "" {
		return
	}
}
`

func TestTheSwitchCensusFollowsHelpersAndSplitsByBuildTag(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"pkg/pages/pages_test.go":       goldenFlagFile,
		"pkg/pages/sweep_test.go":       taggedFile,
		"pkg/pages/sweephelper_test.go": taggedHelperFile,
		"pkg/pages/pages.go":            "package pages\n",
	})
	got, err := goGenerators(root, []string{
		"pkg/pages/pages.go", "pkg/pages/pages_test.go", "pkg/pages/sweep_test.go", "pkg/pages/sweephelper_test.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []goGenerator{
		{
			Dir: "pkg/pages", Tests: []string{"TestThePagesAreCurrent", "TestTheSnapshotsAreCurrent"},
			Flags: []string{"-update-pages"}, Env: []string{"UPDATE_SNAPSHOTS=1"},
		},
		{
			Dir: "pkg/pages", Tests: []string{"TestTheSweepIsPublished"},
			Env: []string{"UPDATE_SWEEP=1"}, Tags: "integration,sweep",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("goGenerators =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestATestCommandRunsFromItsModuleUnderTheRootWorkspace(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"backend/go.mod": "module example.com/backend\n"})
	got, err := goTestGenerator(root, goGenerator{
		Dir: "backend/gates", Tests: []string{"TestA", "TestB"}, Flags: []string{"-update-a"}, Env: []string{"UPDATE_B=1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantArgv := []string{"go", "test", "-count=1", "-run", "^(TestA|TestB)$", "./gates", "-update-a"}
	if got.Dir != "backend" || !reflect.DeepEqual(got.Argv, wantArgv) {
		t.Errorf("command = %s %v, want backend %v", got.Dir, got.Argv, wantArgv)
	}
	if !reflect.DeepEqual(got.Env[1:], []string{"UPDATE_B=1"}) {
		t.Errorf("env = %v, want the root workspace and UPDATE_B=1", got.Env)
	}
}
