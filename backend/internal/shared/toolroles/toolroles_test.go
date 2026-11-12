// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package toolroles

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
)

// sweepFolder is a folder the sweep has written before: it holds a head.
func sweepFolder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, HeadFile), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAFolderWrittenReadsBackTheSameVerbs(t *testing.T) {
	dir := sweepFolder(t)
	want := map[string]Checks{"get_deal": {Verb: "get_deal", Checks: []string{"deal.read"}}, "list_tags": {Verb: "list_tags", Checks: []string{}}}
	files, err := Render(Head{Note: "n"}, want)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, files); err != nil {
		t.Fatal(err)
	}
	head, got, err := Read[Checks](os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if head.Note != "n" || len(got) != 2 || !slices.Equal(got["get_deal"].Checks, []string{"deal.read"}) {
		t.Errorf("read back head %+v and verbs %+v", head, got)
	}
}

func TestAFileNotNamedForItsVerbIsRefused(t *testing.T) {
	fsys := fstest.MapFS{
		HeadFile:        {Data: []byte(`{"note":"n"}`)},
		"get_deal.json": {Data: []byte(`{"verb":"get_contact","checks":[]}`)},
	}
	if _, _, err := Read[Checks](fsys); err == nil {
		t.Error("a file holding another verb was read")
	}
}

func TestDriftNamesEachMissingExtraAndChangedFile(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"kept.json": "a\n", "changed.json": "old\n", "gone.json": "x\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string][]byte{"kept.json": []byte("a\n"), "changed.json": []byte("new\n"), "added.json": []byte("y\n"), HeadFile: []byte("{}\n")}
	if err := os.WriteFile(filepath.Join(dir, HeadFile), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	drift, err := Compare(dir, want)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(drift.Missing, []string{"added.json"}) || !slices.Equal(drift.Extra, []string{"gone.json"}) ||
		!slices.Equal(drift.Changed, []string{"changed.json"}) {
		t.Errorf("drift = %+v", drift)
	}
	if err := Write(dir, want); err != nil {
		t.Fatal(err)
	}
	if after, err := Compare(dir, want); err != nil || !after.Empty() {
		t.Errorf("after a write the folder still drifts: %v, %v", after, err)
	}
}

func TestWriteRefusesAFolderTheSweepDidNotMake(t *testing.T) {
	stray := filepath.Join(t.TempDir(), "elsewhere")
	if err := Write(stray, map[string][]byte{HeadFile: []byte("{}\n")}); err == nil {
		t.Error("a folder with no head was written")
	}
	if _, err := os.Stat(stray); err == nil {
		t.Error("the refused write made the folder anyway")
	}
	dir := sweepFolder(t)
	if err := os.WriteFile(filepath.Join(dir, "get_deal.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, map[string][]byte{}); err == nil {
		t.Error("an empty want was written, emptying the folder")
	}
	if _, err := os.Stat(filepath.Join(dir, "get_deal.json")); err != nil {
		t.Error("the refused write removed a file")
	}
}
