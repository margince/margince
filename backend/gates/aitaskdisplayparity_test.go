// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// TestDisplayTaskKeysMatchTheTaskCatalog holds the API task identity and English UI names to the owning catalog.
func TestDisplayTaskKeysMatchTheTaskCatalog(t *testing.T) {
	t.Parallel()
	declared := crmYAMLNamedEnum(t, "AiTask")
	tasks := ai.AllTasks()
	if len(tasks) == 0 {
		t.Fatal("the task catalog is empty")
	}
	english := frontendCatalog(t, "en")
	want := make([]string, 0, len(tasks))
	for _, task := range tasks {
		want = append(want, string(task))
		if got, want := english["aiTask.name."+string(task)], ai.DisplayName(task); got != want {
			t.Errorf("%s English display name = %q; catalog = %q", task, got, want)
		}
	}
	slices.Sort(want)
	if !slices.Equal(declared, want) {
		t.Fatalf("display task keys %v differ from the task catalog %v", declared, want)
	}
}
