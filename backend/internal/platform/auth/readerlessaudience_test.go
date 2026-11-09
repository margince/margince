// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A job has no reader behind its answer, so it reads what every member may. A
// human's own gate decides for them, so nothing is added.
func TestOnlyTheSystemPrincipalIsNarrowedToTheWorkspaceAudience(t *testing.T) {
	t.Parallel()
	job := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	if got := ReaderlessAudience(job, "a"); got != AudienceWorkspaceOnly("a") {
		t.Errorf("the system principal reads %q, want the workspace audience", got)
	}
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:test"})
	if got := ReaderlessAudience(human, "a"); got != "" {
		t.Errorf("a human is narrowed by %q, want nothing added", got)
	}
}
