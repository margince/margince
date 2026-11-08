// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PromotionDealChoice carries the requested qualification outcome across write surfaces.
type PromotionDealChoice struct {
	ExistingDealID *ids.UUID         `json:"existing_deal_id,omitempty"`
	Deal           *PromotionNewDeal `json:"deal,omitempty"`
	CreateCompany  bool              `json:"create_company,omitempty"`
}

// PromotionNewDeal carries only the optional new opportunity fields.
type PromotionNewDeal struct {
	PipelineID  *ids.UUID `json:"pipeline_id,omitempty"`
	StageID     *ids.UUID `json:"stage_id,omitempty"`
	Name        string    `json:"name,omitempty"`
	AmountMinor *int64    `json:"amount_minor,omitempty"`
	Currency    *string   `json:"currency,omitempty"`
}

// PromotionDealSelection distinguishes the mutually exclusive outcome requests.
type PromotionDealSelection struct {
	New      bool
	Existing bool
}

// ValidatePromotionDeals refuses a request that names both deal outcomes.
func ValidatePromotionDeals(choice PromotionDealSelection) error {
	if choice.New && choice.Existing {
		return &promotionDealConflict{}
	}
	return nil
}

type promotionDealConflict struct{}

func (*promotionDealConflict) Error() string { return "Choose an existing deal or open a new one." }

func (e *promotionDealConflict) FieldFaults() []apperrors.FieldRefusal {
	return []apperrors.FieldRefusal{{Field: "deal", Code: "mutually_exclusive", Message: e.Error()}, {Field: "existing_deal_id", Code: "mutually_exclusive", Message: e.Error()}}
}
