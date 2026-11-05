// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestProgressionPreviewBindsTheActualDealAndSourceStage(t *testing.T) {
	deal, from, to := ids.NewV7(), ids.NewV7(), ids.NewV7()
	payload := map[string]any{"deal_id": deal.String(), "from_stage_id": from.String(), "to_stage_id": to.String(), "lost_reason": "Not executed", "note": "Not executed"}
	approval := crmcontracts.Approval{Kind: deals.StageProgressionKind, TargetEntityType: new("deal"), TargetEntityId: new(crmcontracts.Id(deal)), ProposedChange: &payload}
	current := map[string]json.RawMessage{"stage_id": json.RawMessage(`"` + from.String() + `"`)}
	if err := validateProgressionPreview(approval, current); err != nil {
		t.Fatal(err)
	}
	changes, covered, err := commercialChanges(approval)
	if err != nil || !covered || len(changes) != 1 || string(changes["stage_id"]) != `"`+to.String()+`"` {
		t.Fatalf("changes=%s covered=%t err=%v", changes, covered, err)
	}
	approval.TargetEntityId = new(crmcontracts.Id(ids.NewV7()))
	if err := validateProgressionPreview(approval, current); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("another deal: %v", err)
	}
	approval.TargetEntityId = new(crmcontracts.Id(deal))
	current["stage_id"] = json.RawMessage(`"` + to.String() + `"`)
	if err := validateProgressionPreview(approval, current); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("another source stage: %v", err)
	}
}
