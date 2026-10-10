// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// RecordClaim files a claim through the real writer as ctx, against a message
// that holds the claim's quote. The writer refuses a quote its message does not
// hold, so the fixture adds the quote to the message when it is missing. A
// fixture cannot then file a claim the writer would refuse as ungrounded.
func (e *Env) RecordClaim(ctx context.Context, t *testing.T, in contacts.ClaimInput) crmcontracts.ConversationClaim {
	t.Helper()
	body := e.WsScalar(t, `SELECT COALESCE(body, '') FROM activity WHERE id = $1`, in.ActivityID) //nolint:contextcheck // setup helpers bind the workspace themselves
	if !values.Quoted(body, in.Quote) {
		e.WsExec(t, `UPDATE activity SET body = btrim(COALESCE(body, '') || ' ' || $2::text) WHERE id = $1`, //nolint:contextcheck // setup helpers bind the workspace themselves
			in.ActivityID, in.Quote)
	}
	claim, err := e.Contacts.RecordConversationClaim(ctx, in)
	if err != nil {
		t.Fatalf("recording the claim %q: %v", in.Body, err)
	}
	return claim
}
