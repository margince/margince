// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The egress page and the routing table say the same thing.
//
// docs/reference/ai-egress.md answers "can our mail leave this machine", which
// is a question somebody asks before signing a data-protection agreement. It is
// generated from ai-tasks.yaml precisely so it cannot drift — but a generated
// file only stays true while somebody regenerates it, and a stale one fails in
// the worst direction: it claims a task is local after a ladder gained a cloud
// rung, and the reader acts on a promise the product stopped keeping.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTheEgressPageMatchesTheTaskContract(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("api", "ai-tasks.yaml"))
	if err != nil {
		t.Fatalf("reading the task contract: %v", err)
	}
	var contract struct {
		Tasks map[string]struct {
			Ladder    []string `yaml:"ladder"`
			NoPayload bool     `yaml:"no_payload"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("parsing the task contract: %v", err)
	}
	if len(contract.Tasks) == 0 {
		t.Fatal("the contract declares no tasks — this gate would then pass over nothing")
	}
	page, err := os.ReadFile(filepath.Join("..", "docs", "reference", "ai-egress.md"))
	if err != nil {
		t.Fatalf("reading the egress page: %v; regenerate it with make gen", err)
	}
	text := string(page)

	local := map[string]bool{"local_small": true, "local_large": true}
	for name, def := range contract.Tasks {
		// The row, matched by its leading cell so a task named inside the prose
		// does not satisfy the check for a task missing from the table.
		row, found := rowFor(text, name)
		if !found {
			t.Errorf("task %q is in the contract and not in the egress page — a reader asking "+
				"whether its text leaves the machine finds no answer. Run make gen", name)
			continue
		}
		staysLocal := len(def.Ladder) > 0
		for _, tier := range def.Ladder {
			if !local[tier] {
				staysLocal = false
			}
		}
		if got := cellSaysYes(row, 3); got != staysLocal {
			t.Errorf("the egress page says stays-local=%v for %q; its ladder %v says %v. "+
				"A page claiming more privacy than the routing table delivers is the failure "+
				"this gate exists for. Run make gen", got, name, def.Ladder, staysLocal)
		}
		if got := cellSaysYes(row, 4); got != def.NoPayload {
			t.Errorf("the egress page says prompt-not-retained=%v for %q; the contract says %v. "+
				"Run make gen", got, name, def.NoPayload)
		}
	}
}

// rowFor finds the table row whose FIRST cell names this task.
func rowFor(page, task string) (string, bool) {
	want := "| `" + task + "` |"
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(line, want) {
			return line, true
		}
	}
	return "", false
}

// cellSaysYes reads one cell of a markdown table row, 1-indexed past the
// leading pipe.
func cellSaysYes(row string, n int) bool {
	cells := strings.Split(strings.Trim(row, "| "), "|")
	if n-1 >= len(cells) {
		return false
	}
	return strings.TrimSpace(cells[n-1]) == "yes"
}
