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

func TestARowsFolderReadsEachRowByItsVerb(t *testing.T) {
	fsys := fstest.MapFS{
		HeadFile:        {Data: []byte(`{"note":"n","roles":["admin","sales_rep"]}`)},
		"get_deal.json": {Data: []byte(`{"tool":"deals","verb":"get_deal","allowed_for":["admin"],"refused_for":["sales_rep"]}`)},
	}
	head, rows, err := Read[Row](fsys)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(head.Roles, []string{"admin", "sales_rep"}) || !slices.Equal(rows["get_deal"].Refused, []string{"sales_rep"}) {
		t.Errorf("read head %+v and rows %+v", head, rows)
	}
}

func TestAFolderThatDoesNotReadIsAnError(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"no head":          {"get_deal.json": {Data: []byte(`{"verb":"get_deal"}`)}},
		"head not json":    {HeadFile: {Data: []byte(`{`)}},
		"verb file broken": {HeadFile: {Data: []byte(`{}`)}, "get_deal.json": {Data: []byte(`[`)}},
	} {
		if _, _, err := Read[Checks](fsys); err == nil {
			t.Errorf("%s: the folder was read", name)
		}
	}
}

func TestDriftReadsAsTheFilesThatDiffer(t *testing.T) {
	drift := Drift{Missing: []string{"a.json"}, Changed: []string{"b.json", "c.json"}}
	if got, want := drift.String(), "missing: a.json; changed: b.json, c.json"; got != want {
		t.Errorf("drift reads %q, want %q", got, want)
	}
	if drift.Empty() || !(Drift{}).Empty() {
		t.Error("Empty does not tell a drift from none")
	}
}

func TestAFolderThatCannotBeListedOrReadIsAnError(t *testing.T) {
	want := map[string][]byte{HeadFile: []byte("{}\n")}
	if _, err := Compare(filepath.Join(t.TempDir(), "bad["), want); err == nil {
		t.Error("a folder name that is not a pattern was compared")
	}
	dir := sweepFolder(t)
	if err := os.Mkdir(filepath.Join(dir, "unreadable.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Compare(dir, want); err == nil {
		t.Error("a verb file that cannot be read was compared")
	}
	if err := Write(dir, want); err == nil {
		t.Error("a folder holding a verb file that cannot be read was written")
	}
}
