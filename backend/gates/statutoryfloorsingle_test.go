// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The statutory retention floor is spelled once, and every destructive activity
// path applies that one spelling.
//
// The predicate's own comment states the consequence of a path skipping it:
// correspondence the nightly evaluator refuses to touch for six years gets
// destroyed anyway — a GoBD floor bypass. That already happened once: the
// capture purge shipped its first draft with no floor at all, and a review
// caught it before merge. A second SPELLING would be the same defect arriving
// more quietly, because the two texts would drift and only one of them would be
// wrong.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheStatutoryFloorIsSpelledOnce(t *testing.T) {
	t.Parallel()
	// The floor is recognisable by the window comparison at its heart: the
	// instant the statutory period closes, compared against now(). A second
	// implementation would have to write that comparison again.
	const windowTest = "> now()"
	const handelsbrief = "retention_class IS NOT NULL"

	var spellings []string
	root := filepath.Join("internal")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		if strings.Contains(text, handelsbrief) && strings.Contains(text, windowTest) {
			spellings = append(spellings, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module tree: %v", err)
	}
	if len(spellings) == 0 {
		t.Fatal("no file spells the statutory floor — this gate reads the tree by shape, " +
			"so a rename must re-point it rather than leave it passing over nothing")
	}
	if len(spellings) != 1 {
		t.Errorf("the statutory floor is spelled in %d files (%v), want exactly 1 — "+
			"a destructive path that carries its own copy is one that stops shielding what the "+
			"others do, and the drift is invisible until somebody's correspondence is gone",
			len(spellings), spellings)
	}
}
