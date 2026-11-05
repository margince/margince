// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// The sender read arm opens once a step of this name is done. No other task in
// the worker catalog may use that name.
func TestOnlyTheColleagueSenderTaskOpensTheSenderReadArm(t *testing.T) {
	t.Parallel()
	var owners []string
	for _, task := range dataTaskCatalog(nil, nil) {
		for _, step := range task.Steps {
			if step.Name == auth.SenderStampsCheckedStep {
				owners = append(owners, task.ID)
			}
		}
	}
	if want := colleagueSenderDataTask().ID; len(owners) != 1 || owners[0] != want {
		t.Fatalf("tasks with the step %q: %v, want only %s", auth.SenderStampsCheckedStep, owners, want)
	}
}
