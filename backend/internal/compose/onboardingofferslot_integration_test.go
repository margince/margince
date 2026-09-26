// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The offer slot's own guarantees against the real site_read row: an offer
// belongs to the administrator it was made to, a masked field is never
// offered, and one offer is accepted at most once however the answers race.

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const slotOfferSQL = `SELECT coalesce(conversation_offer::text, '') FROM site_read WHERE id = $1`

func displayNameOffer(read contacts.SiteRead) *contacts.SiteReadOffer {
	offer := recordedOffer(companyReadOffer{Field: fieldDisplayName, Value: "Acme", SourceIDs: []string{"S1"}},
		offeringMessage, read.DraftVersion)
	return &offer
}

func TestAnOfferMadeToOneAdministratorGrantsAnotherNothing(t *testing.T) {
	env := integration.Setup(t)
	read := onboardingDraft(t, env)
	offeredTo := env.As(env.Rep1, nil, integration.AdminPerms)
	other := env.As(env.Rep2, nil, integration.AdminPerms)
	if err := env.Contacts.ReplaceSiteReadOffer(offeredTo, read.ID, displayNameOffer(read), nil); err != nil {
		t.Fatalf("record the offer: %v", err)
	}
	before := env.WsScalar(t, slotOfferSQL, read.ID)

	if standing, err := env.Contacts.StandingSiteReadOffer(other, read.ID); err != nil || standing != nil {
		t.Fatalf("the other administrator reads %+v (%v), want no offer", standing, err)
	}
	engine := &deepReadEngine{
		contacts: env.Contacts, runtime: ai.NewRunTransparency(env.DB()),
		brain: &replyBrainStub{response: model.Response{Text: acceptingReply}},
	}
	history := []crmcontracts.CompanySiteReadConversationTurn{
		{Role: crmcontracts.CompanySiteReadConversationTurnRoleUser, Message: offeringQuestion},
		{Role: crmcontracts.CompanySiteReadConversationTurnRoleAssistant, Message: offeringMessage},
	}
	recorder := httptest.NewRecorder()
	engine.messageCompanySiteRead(recorder, offerMessageRequest(other, t, read.ID.String(), "Yes", history), openapi_types.UUID(read.ID))
	if changes := proposedBy(t, recorder); len(changes) != 0 {
		t.Fatalf("the other administrator's yes was granted %+v", changes)
	}

	if after := env.WsScalar(t, slotOfferSQL, read.ID); after != before {
		t.Fatalf("the other administrator's yes rewrote the slot:\nbefore %s\nafter  %s", before, after)
	}
	if standing, err := env.Contacts.StandingSiteReadOffer(offeredTo, read.ID); err != nil || standing == nil {
		t.Fatalf("the offer no longer stands for the administrator it was made to: %+v (%v)", standing, err)
	}
}

func TestAMaskedFieldIsNeverOffered(t *testing.T) {
	env := integration.Setup(t)
	read := onboardingDraft(t, env)
	// A row-scope-all reader carries no mask at all, so this one is team-scoped.
	perms := integration.AdminPerms
	perms.RowScope = principal.RowScopeTeam
	perms.FieldMasks = []principal.FieldMask{{Object: "company", Field: fieldDisplayName}}
	masked := env.As(env.Rep1, nil, perms)

	err := env.Contacts.ReplaceSiteReadOffer(masked, read.ID, displayNameOffer(read), nil)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("offering a field masked from its reader = %v, want permission denied", err)
	}
	if slot := env.WsScalar(t, slotOfferSQL, read.ID); slot != "" {
		t.Fatalf("a refused offer reached the slot: %s", slot)
	}
}

func TestTwoAnswersToOneOfferAcceptItOnce(t *testing.T) {
	env := integration.Setup(t)
	read := onboardingDraft(t, env)
	human := env.As(env.Rep1, nil, integration.AdminPerms)
	if err := env.Contacts.ReplaceSiteReadOffer(human, read.ID, displayNameOffer(read), nil); err != nil {
		t.Fatalf("record the offer: %v", err)
	}
	standing, err := env.Contacts.StandingSiteReadOffer(human, read.ID)
	if err != nil || standing == nil {
		t.Fatalf("the recorded offer does not stand: %+v (%v)", standing, err)
	}

	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(ctx context.Context) {
			defer wg.Done()
			results[i] = env.Contacts.ReplaceSiteReadOffer(ctx, read.ID, nil, standing)
		}(human)
	}
	wg.Wait()

	accepted, conflicted := 0, 0
	for _, result := range results {
		switch {
		case result == nil:
			accepted++
		case errors.Is(result, apperrors.ErrConflict):
			conflicted++
		default:
			t.Fatalf("an answer to the offer failed: %v", result)
		}
	}
	if accepted != 1 || conflicted != 1 {
		t.Fatalf("%d answers accepted the offer and %d conflicted, want 1 and 1", accepted, conflicted)
	}
}
