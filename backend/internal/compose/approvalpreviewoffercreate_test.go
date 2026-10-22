// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOfferCreationRequiresTheCanonicalParentRoute(t *testing.T) {
	parent := ids.NewV7()
	for _, path := range []string{"", "/v1/offers/" + parent.String(), "/v1/deals/" + parent.String() + "/offers/extra", "/v1/deals/00000000-0000-0000-0000-000000000000/offers"} {
		payload := map[string]any{"operation": "createOffer", "path": path, "body": map[string]any{"currency": "EUR", "source": "manual"}}
		_, _, err := offerCreationRequest(crmcontracts.Approval{TargetEntityType: new("offer"), ProposedChange: &payload})
		if !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("path %q: %v", path, err)
		}
	}
	raw, _, err := canonicalRESTCall("createOffer", "/v1/deals/"+parent.String()+"/offers", nil, []byte(`{"currency":"EUR","source":"manual"}`), keyBindsTheRetry)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	got, input, err := offerCreationRequest(crmcontracts.Approval{TargetEntityType: new("offer"), ProposedChange: &payload})
	if err != nil || got.UUID != parent || input.Currency != "EUR" {
		t.Fatalf("canonical parent lost: %v / %+v / %v", got, input, err)
	}
	payload["operation"] = "createDeal"
	if _, _, err := offerCreationRequest(crmcontracts.Approval{TargetEntityType: new("offer"), ProposedChange: &payload}); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("unrelated operation admitted: %v", err)
	}
}
