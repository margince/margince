// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

func TestTheAgentSeamRefusesAnIDOfAnotherKind(t *testing.T) {
	person := authz.HumanPrincipal(ids.NewV7(), authz.RBAC{}, principal.SeatFull)
	for _, id := range []string{"system:fx", "connector:surfe", "human:" + person.UserID.String(), "agent:", "margince"} {
		if _, err := agentPrincipalFor(person, id); err == nil {
			t.Errorf("agentPrincipalFor(%q) minted an agent; an id of another kind writes an audit row naming two actors", id)
		}
	}
	agent, err := agentPrincipalFor(person, marginceActorID)
	if err != nil || agent.Type != principal.PrincipalAgent || agent.ID != marginceActorID {
		t.Errorf("agentPrincipalFor(%q) = %+v, %v; want an agent under that id", marginceActorID, agent, err)
	}
}
