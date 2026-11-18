// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheAgentClientListReadsBackItsOwnNextCursor(t *testing.T) {
	listed := []listedClient{
		{AgentClient: AgentClient{ID: ids.NewV7(), CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, tier: 1},
		{AgentClient: AgentClient{ID: ids.NewV7(), CreatedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)}, tier: 1},
	}
	_, page, err := agentClientPage(listed, 1)
	if err != nil || page.NextCursor == "" {
		t.Fatalf("page = %+v (err %v), want a next cursor", page, err)
	}
	got, err := decodeAgentClientCursor(&page.NextCursor)
	if err != nil || got.id == nil || *got.id != listed[0].ID {
		t.Fatalf("the list refused its own cursor: %+v, %v", got, err)
	}
}
