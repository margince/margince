// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// A first message is written from more than its links: the deal and the
// conversation the engine folded in beside the recipient. Its retry re-proves
// every one of them, so losing sight of a record the caller never linked still
// withholds the text derived from it.
func TestAFirstDraftsReplayIsRefusedOnceARecordItWasWrittenFromIsOutOfReach(t *testing.T) {
	contact, deal, conversation := ids.NewV7(), ids.NewV7(), ids.NewV7()
	comms := &recordingComms{grounding: []EvidenceRef{
		{RecordType: datasource.EntityContact, RecordID: contact},
		{RecordType: datasource.EntityDeal, RecordID: deal},
		{RecordType: datasource.EntityActivity, RecordID: conversation},
	}}
	claims := &recordingClaims{verdict: Claim{State: ClaimFresh, Attempt: freshAttempt}}
	reader := &answeringReader{}
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}),
		WithIdempotency(claims), WithReplayReader(reader), WithVolumeCharger(newCountingCharger()))
	registry.Register(draftEmailTool{comms: comms, p: oneRecord(datasource.EntityContact, contact, `{}`, 1)})
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:t", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeDraft),
	})
	call := json.RawMessage(`{"idempotency_key":"k-1","intent":"follow up",` +
		`"links":[{"entity_type":"contact","entity_id":"` + contact.String() + `"}]}`)

	if _, err := registry.Invoke(ctx, "draft_email", call); err != nil {
		t.Fatalf("the first draft: %v", err)
	}
	claims.verdict = Claim{State: ClaimReplay, Result: claims.stored, Records: claims.storedRecords}
	if _, err := registry.Invoke(ctx, "draft_email", call); err != nil {
		t.Fatalf("a replay while every record is visible: %v", err)
	}
	if reader.reads != 3 {
		t.Errorf("the replay probed %d records, want the 3 the draft was written from", reader.reads)
	}

	comms.accountDrafted = nil
	reader.deny = map[ids.UUID]error{conversation: apperrors.ErrNotFound}
	out, err := registry.Invoke(ctx, "draft_email", call)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a replay after an unlinked conversation went out of reach → %v (%s), want ErrNotFound", err, out)
	}
	if comms.accountDrafted != nil {
		t.Error("the refused replay reached the drafting engine; a retry must never draft again")
	}
}
