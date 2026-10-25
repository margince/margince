// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A name learned later must still replace the task's rewrite. So the guard in
// contacts admits the actor the executor binds for this task.
func TestTheInitialsTaskBindsTheActorTheNameGuardAdmits(t *testing.T) {
	t.Parallel()
	actor, ok := principal.Actor(datatask.Acting(context.Background(), initialsNamesDataTask().ID))
	if !ok || actor.ID != contacts.InitialsRepairActor {
		t.Errorf("the task's batches act as %q, want contacts.InitialsRepairActor %q", actor.ID, contacts.InitialsRepairActor)
	}
}
