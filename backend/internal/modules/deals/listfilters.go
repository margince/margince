// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What an enumeration of this module's records may be narrowed by — the deal
// and project halves of the same rule contacts/listfilters.go states: the names
// are the contract's list-operation parameters, and this file says which of
// them this store answers and what each one narrows.

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The wire field names this module answers, spelled once each: a caller's
// query parameter on a list, and the same name where a read reports the field
// withheld (fieldmask.go). They are wire names, which is why they are not the
// column constants they happen to match today.
const (
	filterCompanyID          = "company_id"
	filterTag                = "tag_id"
	filterTagMode            = "tag_mode"
	filterOwnerID            = "owner_id"
	filterPartnerCompanyID   = "partner_company_id"
	filterPartnerAttribution = "partner_attribution"
	filterPartnerSourced     = "partner_sourced"
	filterPipelineID         = "pipeline_id"
	filterProjectID          = "project_id"
	filterStageID            = "stage_id"
	filterStalled            = "stalled"
	filterStatus             = "status"
	filterForecastCategory   = "forecast_category"
	filterKey                = "key"
	filterPhase              = "phase"
	filterCommercialMotion   = "commercial_motion"
	filterPriority           = "priority"
	filterAcquisitionSource  = "acquisition_source"
)

// filterUnset is the sentinel the three commercial-context filters take to
// mean "no value recorded". A filter parameter cannot say that with an enum
// member, and an empty string already means "not filtering".
const filterUnset = "unset"

var dealListFilters = storekit.FilterSet[ListDealsInput]{
	filterTag: storekit.FilterIDList[ids.TagKind](func(in *ListDealsInput, v []ids.UUID) { in.TagIDs = v }),
	filterTagMode: storekit.FilterWord(func(in *ListDealsInput, v *string) {
		// A stored mode the enum does not admit selects `any`, the contract's
		// default for an absent one: a saved view has no caller to refuse to.
		mode, err := storekit.ParseTagMode(v)
		if err != nil {
			mode = storekit.TagModeAny
		}
		in.TagMode = mode
	}),
	filterCompanyID: storekit.FilterID(
		func(in *ListDealsInput, id *ids.CompanyID) { in.CompanyID = id }),
	filterOwnerID: storekit.FilterID(func(in *ListDealsInput, id *ids.UserID) { in.OwnerID = id }),
	filterPartnerCompanyID: storekit.FilterID(
		func(in *ListDealsInput, id *ids.CompanyID) { in.PartnerCompanyID = id }),
	filterPartnerAttribution: storekit.FilterWord(
		func(in *ListDealsInput, v *string) { in.PartnerAttribution = v }),
	filterPartnerSourced: storekit.FilterFlag(func(in *ListDealsInput, v *bool) { in.PartnerSourced = v }),
	filterPipelineID:     storekit.FilterID(func(in *ListDealsInput, id *ids.PipelineID) { in.PipelineID = id }),
	filterProjectID:      storekit.FilterID(func(in *ListDealsInput, id *ids.ProjectID) { in.ProjectID = id }),
	filterStageID:        storekit.FilterID(func(in *ListDealsInput, id *ids.StageID) { in.StageID = id }),
	filterStalled:        storekit.FilterFlag(func(in *ListDealsInput, v *bool) { in.Stalled = v }),
	filterStatus:         storekit.FilterWord(func(in *ListDealsInput, v *string) { in.Status = v }),
	filterForecastCategory: storekit.FilterWord(
		func(in *ListDealsInput, v *string) { in.ForecastCategory = v }),
	filterCommercialMotion: storekit.FilterWord(
		func(in *ListDealsInput, v *string) { in.CommercialMotion = v }),
	filterPriority: storekit.FilterWord(func(in *ListDealsInput, v *string) { in.Priority = v }),
	filterAcquisitionSource: storekit.FilterWord(
		func(in *ListDealsInput, v *string) { in.AcquisitionSource = v }),
}

// ListFilters names what SearchEntity can narrow one entity type by.
func (p *Provider) ListFilters(t datasource.EntityType) []string {
	switch t {
	case datasource.EntityDeal:
		return dealListFilters.Names()
	default:
		return nil
	}
}
