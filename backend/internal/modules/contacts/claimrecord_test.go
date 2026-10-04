// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An extracted claim is grounded or absent, and the writer names which half is
// missing before it asks who is writing.
func TestAnExtractedClaimIsRefusedWithoutItsGround(t *testing.T) {
	grounded := ClaimInput{Body: "Send pricing", ActivityID: ids.NewV7(), Quote: "I'll send pricing."}
	for _, c := range []struct {
		name  string
		input func(ClaimInput) ClaimInput
	}{
		{"no body", func(in ClaimInput) ClaimInput { in.Body = ""; return in }},
		{"no source message", func(in ClaimInput) ClaimInput { in.ActivityID = ids.UUID{}; return in }},
		{"no quote", func(in ClaimInput) ClaimInput { in.Quote = ""; return in }},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := (&Store{}).RecordConversationClaimTx(context.Background(), nil, c.input(grounded)); err == nil {
				t.Errorf("a claim with %s was accepted", c.name)
			}
		})
	}
}

// An agent passport may not file a claim, and a writer without contact:update
// may not either — the claim writes to a contact's record.
func TestAnExtractedClaimNeedsAWriterAllowedToFileIt(t *testing.T) {
	grounded := ClaimInput{Body: "Send pricing", ActivityID: ids.NewV7(), Quote: "I'll send pricing."}
	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:somebody",
	})
	reader := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(),
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"contact": {Read: true}}},
	})
	for name, ctx := range map[string]context.Context{"an agent": agent, "a reader": reader} {
		_, _, err := (&Store{}).RecordConversationClaimTx(ctx, nil, grounded)
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s filing a claim: err = %v, want permission denied", name, err)
		}
	}
	if err := (&Store{}).SetClaimTaskTx(reader, nil, ids.NewV7(), ids.NewV7()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader linking a claim to a task: err = %v, want permission denied", err)
	}
	if _, _, err := (&Store{}).ContactNamedAmong(agent, nil, []ids.UUID{ids.NewV7()}, "Ines"); err == nil {
		t.Error("an actor with no contact grant looked a contact up by name")
	}
	if _, found, err := (&Store{}).ContactNamedAmong(reader, nil, nil, "Ines"); found || err != nil {
		t.Errorf("a name among no contacts: found=%v err=%v, want nothing and no error", found, err)
	}
}
