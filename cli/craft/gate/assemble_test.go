package gate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssemble_gathersDiffTouchedAndSiblingFiles(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("crm/crm-core/handler_person.go", "package crmcore\nfunc Person() {}\n")
	write("crm/crm-core/handler_deal.go", "package crmcore\nfunc Deal() {}\n")
	write("crm/crm-core/zz_gen.go", "// generated; must be skipped\n")
	write("AGENTS.md", "# root\n## Craftsmanship\nrules\n")

	a := &Assembler{Root: root, Git: func(_ context.Context, _ string, args ...string) (string, error) {
		switch {
		case args[0] == "diff" && contains(args, "--name-only"):
			return "crm/crm-core/handler_person.go", nil
		case args[0] == "diff":
			return "@@ a fake unified diff @@", nil
		}
		return "", nil
	}}

	in, err := a.Assemble(context.Background(), "main", "HEAD")
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if in.Diff == "" {
		t.Error("diff not captured")
	}
	if _, ok := in.TouchedFiles["crm/crm-core/handler_person.go"]; !ok {
		t.Error("touched file content not captured")
	}
	if _, ok := in.SiblingFiles["crm/crm-core/handler_deal.go"]; !ok {
		t.Error("sibling file not captured")
	}
	if _, ok := in.SiblingFiles["crm/crm-core/zz_gen.go"]; ok {
		t.Error("generated _gen.go must be skipped as a sibling")
	}
	if in.ModuleAGENTS == "" {
		t.Error("nearest AGENTS.md not captured")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// A directory may hold an AGENTS.md that carries its own rules and no
// Craftsmanship section — frontend/ does. The gate prompt built from that file
// would contain no rubric, and the gate would still return a verdict: one
// reached without the rules, reported exactly like one reached with them.
//
// This also pins the ORDER. dirs is a map and a real diff touches several
// directories, so ranging over it picks a random start and the same diff could
// be judged against a different rulebook on each run.
func TestNearestAgentsSkipsARulebookWithNoRubric(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# root\n\n## Craftsmanship\n\nthe rubric lives here\n")
	writeFile(t, root, "frontend/AGENTS.md", "# frontend\n\nrules with no rubric section\n")
	writeFile(t, root, "frontend/src/app.tsx", "export const a = 1\n")
	writeFile(t, root, "backend/main.go", "package main\n")

	a := &Assembler{Root: root}

	// Walking up from the frontend directory must pass frontend/AGENTS.md and
	// reach the root, because only the root file carries the rubric.
	got := a.nearestAgents(map[string]bool{"frontend/src": true})
	if !strings.Contains(got, "the rubric lives here") {
		t.Errorf("walking up from frontend/src returned a rulebook with no ## Craftsmanship section:\n%s\n"+
			"The gate would run with no rubric in its prompt and still return a verdict.", got)
	}

}

// The order the touched directories are walked in has to be fixed, and proving
// that needs TWO rulebooks that both carry the rubric — the case the prompt's own
// label ("## Craftsmanship deltas") anticipates, a module holding deltas of its
// own. With only one rubric-bearing file the answer is order-independent for the
// wrong reason, and a sort could be deleted with nothing going red.
func TestNearestAgentsDoesNotDependOnMapOrder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# root\n\n## Craftsmanship\n\nthe root rubric\n")
	writeFile(t, root, "extensions/notes/AGENTS.md", "# notes\n\n## Craftsmanship\n\nthe notes deltas\n")
	writeFile(t, root, "extensions/notes/notes.go", "package notes\n")
	writeFile(t, root, "backend/main.go", "package main\n")

	a := &Assembler{Root: root}
	dirs := map[string]bool{"extensions/notes": true, "backend": true}

	first := a.nearestAgents(dirs)
	if first == "" {
		t.Fatal("no rulebook found at all")
	}
	// Map iteration is randomised per range, so a walk that depends on it
	// disagrees with itself across enough attempts.
	for i := 0; i < 50; i++ {
		if got := a.nearestAgents(dirs); got != first {
			t.Fatalf("iteration %d returned a different rulebook than the first call — the walk depends on map "+
				"iteration order, so the same diff is judged against a different rubric run to run.\n"+
				"first:\n%s\nnow:\n%s", i, first, got)
		}
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
