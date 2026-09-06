// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// What .coderabbit.yaml tells the reviewer about backend Go, held against what
// is true.
//
// The reviewer twice filed a blocking finding citing a repository rule
// "backend/**/*.go: Never edit." and asking for the revert of a correct change.
// No such rule existed anywhere in the tree. The fix was to state the truth in
// the config, where a public contributor can read it — and a stated truth is
// only worth having while it stays true.
//
// So this holds the two claims that instruction makes. It does NOT read the
// prose: what the reviewer is told is a sentence, and a gate matching sentences
// would fail on a rewording and pass on a lie. It reads the PATHS the sentence
// names, and checks each against the rule that actually makes it off-limits.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const coderabbitConfig = ".coderabbit.yaml"

func TestTheReviewerIsToldWhichPathsAreReallyOffLimits(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join(repoRoot, coderabbitConfig))
	if err != nil {
		t.Fatalf("reading %s: %v", coderabbitConfig, err)
	}
	config := string(raw)

	if !strings.Contains(config, "path_instructions:") {
		t.Fatal(coderabbitConfig + " carries no path_instructions, so the reviewer has nothing " +
			"true to read about backend Go and is free to infer a prohibition again — which is " +
			"what it did, twice, on correct changes")
	}

	// Each claim, and the rule in the tree that makes it true. A path named
	// here that stops being off-limits is a lie the reviewer will act on; one
	// that becomes off-limits and is not named is a finding nobody was warned
	// about.
	for claim, held := range map[string]func() bool{
		"**/*_gen.go": func() bool {
			return strings.Contains(rulebook(t), "`internal/contracts/` and `*_gen.go` are generated")
		},
		"backend/internal/contracts/**": func() bool {
			return strings.Contains(rulebook(t), "`internal/contracts/` and `*_gen.go` are generated")
		},
		"backend/migrations/core/": func() bool {
			return strings.Contains(rulebook(t), "A shipped migration in `migrations/core/`")
		},
	} {
		if !strings.Contains(config, claim) {
			t.Errorf("%s no longer tells the reviewer that %s is off-limits, and the rulebook still "+
				"says it is — a finding on it would arrive with no warning behind it",
				coderabbitConfig, claim)
		}
		if !held() {
			t.Errorf("%s tells the reviewer %s is off-limits and the rulebook no longer says so — "+
				"the instruction has become the thing this gate exists to prevent, a rule that is "+
				"only written where nobody can check it", coderabbitConfig, claim)
		}
	}
}

// rulebook is AGENTS.md, which every harness reads and no directory duplicates.
//
// Held by: TestEveryRulebookHasAClaudeShim (backend/gates/rulebookdelegation_test.go)
// — a second copy fails there, which is what lets this read one file and call it
// the rules.
func rulebook(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "AGENTS.md"))
	if err != nil {
		t.Fatalf("reading the rulebook: %v", err)
	}
	return string(raw)
}
