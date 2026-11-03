// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedHistoricalLeadAtContactEmail(t *testing.T, e *integration.Env, ctx context.Context, in contacts.CreateLeadInput, email string) ids.LeadID {
	t.Helper()
	in.Email = nil
	lead, _, err := e.Contacts.CreateLead(ctx, in)
	if err != nil {
		t.Fatalf("creating historical lead: %v", err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	e.WsExec(t, `UPDATE lead SET email = lower($2) WHERE id = $1`, id, email)
	return id
}
