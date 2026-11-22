// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An edit access request is put to the owner, and an agent files it for the
// colleague asking, as a handover's. An agent for anyone else is refused.
func TestAnAgentFilesAnEditAccessRequestForItsOwnUser(t *testing.T) {
	owner, requester := ids.NewV7(), ids.NewV7()
	body, err := json.Marshal(approvalsubject.EditAccess{RecordType: "lead", RecordID: ids.NewV7(), OwnerID: owner, RequesterID: requester})
	if err != nil {
		t.Fatal(err)
	}
	in := StageInput{Kind: approvalsubject.KindEditAccess, ProposedChange: body}
	agentFor := func(user ids.UUID) principal.Principal {
		return principal.Principal{Type: principal.PrincipalAgent, PassportID: ids.NewV7(), OnBehalfOf: user, UserID: user}
	}
	if err := payloadNamesItsOwner(agentFor(requester), in); err != nil {
		t.Errorf("the asker's agent filing their request: %v, want admitted", err)
	}
	if err := payloadNamesItsOwner(agentFor(ids.NewV7()), in); !errors.Is(err, ErrNoDecider) {
		t.Errorf("another member's agent filing it: %v, want refused", err)
	}
}
