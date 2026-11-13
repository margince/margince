// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// repository makes a git repository holding files, all committed. Git reads
// no config of the caller's, so a global commit.gpgsign cannot fail it.
func repository(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	root := t.TempDir()
	writeFiles(t, root, files)
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "."},
		{"-c", "user.name=gate", "-c", "user.email=gate@example.invalid", "commit", "-q", "-m", "fixture"},
	} {
		if _, err := gitOut(root, nil, args...); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func shell(name, script string) generator {
	return generator{Name: name, Dir: ".", Argv: []string{"sh", "-c", script}}
}

func planFor(outputs []string, generators ...generator) plan {
	return plan{generators: generators, outputs: outputs, unrun: map[string]bool{}, verified: map[string]string{}}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAGeneratorThatWritesTheSameBytesTwicePassesAndTheTreeIsPutBack(t *testing.T) {
	root := repository(t, map[string]string{"table_gen.txt": "committed\n"})
	err := twice(root, planFor([]string{"table_gen.txt"},
		shell("table", "printf 'fresh\\n' > table_gen.txt && printf 'x' > scratch.txt")), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("a generator that writes the same bytes each run failed the gate: %v", err)
	}
	if got := readFile(t, filepath.Join(root, "table_gen.txt")); got != "committed\n" {
		t.Errorf("the gate left table_gen.txt as %q, want the bytes it found", got)
	}
	if _, err := os.Stat(filepath.Join(root, "scratch.txt")); !os.IsNotExist(err) {
		t.Errorf("the gate left a file a generator added: %v", err)
	}
}

func TestTwoRunsThatDisagreeFailAndNameTheFile(t *testing.T) {
	root := repository(t, map[string]string{"table_gen.txt": "committed\n", "count": "0"})
	// Each run writes a different number: the shape of output that reads map
	// order or the clock.
	counter := shell("counter", "n=$(($(cat count) + 1)); printf $n > count; printf $n > table_gen.txt")
	err := twice(root, planFor([]string{"table_gen.txt"}, counter), log.New(io.Discard, "", 0))
	if err == nil || !strings.Contains(err.Error(), "different bytes") || !strings.Contains(err.Error(), "table_gen.txt") {
		t.Fatalf("two runs that wrote different bytes gave %v, want a failure naming table_gen.txt", err)
	}
	if got := readFile(t, filepath.Join(root, "count")); got != "0" {
		t.Errorf("the gate left count as %q after a failure, want the bytes it found", got)
	}
}

func TestAMarkedFileNoGeneratorWritesFailsTheCensus(t *testing.T) {
	root := repository(t, map[string]string{"orphan_gen.txt": "x\n", "table_gen.txt": "y\n"})
	err := twice(root, planFor([]string{"orphan_gen.txt", "table_gen.txt"},
		shell("table", "printf 'y\\n' > table_gen.txt")), log.New(io.Discard, "", 0))
	if err == nil || !strings.Contains(err.Error(), "orphan_gen.txt") || strings.Contains(err.Error(), "table_gen.txt") {
		t.Fatalf("a marked file no generator wrote gave %v, want a failure naming orphan_gen.txt alone", err)
	}
}

func TestAMarkedFileItsToolNamesNeedNotBeRewritten(t *testing.T) {
	root := repository(t, map[string]string{"units/a/manifest_gen.json": "{}\n"})
	p := planFor([]string{"units/a/manifest_gen.json"}, shell("noop", "true"))
	p.named = map[string]string{"manifest_gen.json": "gen-units"}
	if err := twice(root, p, log.New(io.Discard, "", 0)); err != nil {
		t.Fatalf("a file its tool names and keeps unchanged failed the gate: %v", err)
	}
	if p.verified["units/a/manifest_gen.json"] != "gen-units" {
		t.Errorf("the report does not name gen-units for the manifest: %v", p.verified)
	}
}

func TestAnEditTheDeveloperHadNotCommittedSurvivesTheGate(t *testing.T) {
	root := repository(t, map[string]string{"table_gen.txt": "committed\n"})
	writeFiles(t, root, map[string]string{"table_gen.txt": "edited\n", "notes.txt": "mine\n"})
	err := twice(root, planFor([]string{"table_gen.txt"},
		shell("table", "printf 'fresh\\n' > table_gen.txt; rm -f notes.txt")), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(root, "table_gen.txt")); got != "edited\n" {
		t.Errorf("the gate left table_gen.txt as %q, want the uncommitted edit", got)
	}
	if got := readFile(t, filepath.Join(root, "notes.txt")); got != "mine\n" {
		t.Errorf("the gate left notes.txt as %q, want the untracked file back", got)
	}
}

func TestAFileChangedBeforeTheLastPassIsLeftAndNamed(t *testing.T) {
	root := repository(t, map[string]string{"table_gen.txt": "committed\n", "notes.txt": "mine\n"})
	tree, err := newWorktree(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	before, err := tree.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, map[string]string{"notes.txt": "typed during the first pass\n"})
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, "notes.txt"), old, old); err != nil {
		t.Fatal(err)
	}
	last := lastPass{started: passStart(time.Now())}
	writeFiles(t, root, map[string]string{"table_gen.txt": "fresh\n"})
	if last.tree, err = tree.snapshot(); err != nil {
		t.Fatal(err)
	}
	err = tree.restore(before, last, func(path string) bool { return path == "table_gen.txt" })
	if err == nil || !strings.Contains(err.Error(), "notes.txt") {
		t.Errorf("restore over an edit made in the first pass gave %v, want a failure naming notes.txt", err)
	}
	if got := readFile(t, filepath.Join(root, "notes.txt")); got != "typed during the first pass\n" {
		t.Errorf("restore overwrote the edit made in the first pass: %q", got)
	}
	if got := readFile(t, filepath.Join(root, "table_gen.txt")); got != "committed\n" {
		t.Errorf("restore left the generated file as %q, want the bytes it found", got)
	}
}

func TestAFileChangedAfterTheGeneratorsRanIsLeftAndNamed(t *testing.T) {
	root := repository(t, map[string]string{"table_gen.txt": "committed\n", "notes.txt": "mine\n"})
	tree, err := newWorktree(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	before, err := tree.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	last := lastPass{started: passStart(time.Now())}
	writeFiles(t, root, map[string]string{"table_gen.txt": "fresh\n"})
	if last.tree, err = tree.snapshot(); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, map[string]string{"notes.txt": "typed during the run\n"})
	err = tree.restore(before, last, func(path string) bool { return path == "table_gen.txt" })
	if err == nil || !strings.Contains(err.Error(), "notes.txt") {
		t.Errorf("restore over an edit made during the run gave %v, want a failure naming notes.txt", err)
	}
	if got := readFile(t, filepath.Join(root, "notes.txt")); got != "typed during the run\n" {
		t.Errorf("restore overwrote the edit made during the run: %q", got)
	}
	if got := readFile(t, filepath.Join(root, "table_gen.txt")); got != "committed\n" {
		t.Errorf("restore left the generated file as %q, want the bytes it found", got)
	}
}

func TestAnOutputWrittenOnlyWhenStaleIsPutBack(t *testing.T) {
	root := repository(t, map[string]string{"manifest_gen.json": "stale\n"})
	// The shape of a generator that writes only bytes that change: the first
	// pass rewrites the stale file, the second finds it current. The old time
	// stands for a first pass well before the second.
	conditional := shell("units", `[ "$(cat manifest_gen.json)" = current ] || `+
		`{ printf current > manifest_gen.json; touch -t 200501010000 manifest_gen.json; }`)
	if err := twice(root, planFor([]string{"manifest_gen.json"}, conditional), log.New(io.Discard, "", 0)); err != nil {
		t.Fatalf("an output its generator wrote only in the first pass failed the gate: %v", err)
	}
	if got := readFile(t, filepath.Join(root, "manifest_gen.json")); got != "stale\n" {
		t.Errorf("the gate left manifest_gen.json as %q, want the bytes it found", got)
	}
}

func TestAnEditDuringTheRunIsKeptAndTheOtherOutputsGetTheirTimesBack(t *testing.T) {
	root := repository(t, map[string]string{"table_gen.txt": "committed\n", "notes.txt": "mine\n"})
	original := time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(root, "table_gen.txt"), original, original); err != nil {
		t.Fatal(err)
	}
	once := filepath.Join(t.TempDir(), "edited")
	// The first pass stands in for a developer who saves notes.txt while the
	// gate runs. The old time puts the save well before the second pass.
	editor := shell("editor", `printf 'committed\n' > table_gen.txt; [ -e "`+once+`" ] || `+
		`{ printf typed > notes.txt; touch -t 200501010000 notes.txt; touch "`+once+`"; }`)
	err := twice(root, planFor([]string{"table_gen.txt"}, editor), log.New(io.Discard, "", 0))
	var kept keptEdits
	if !errors.As(err, &kept) || !slices.Equal(kept, keptEdits{"notes.txt"}) {
		t.Fatalf("an edit during the run gave %v, want it kept and named", err)
	}
	if got := readFile(t, filepath.Join(root, "notes.txt")); got != "typed" {
		t.Errorf("the gate overwrote the edit made during the run: %q", got)
	}
	info, err := os.Stat(filepath.Join(root, "table_gen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(original) {
		t.Errorf("table_gen.txt kept the time %v after a kept edit, want its own %v back", info.ModTime(), original)
	}
}
