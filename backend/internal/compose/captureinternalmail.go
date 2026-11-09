// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func internalMailPatch(req *crmcontracts.UpdateInternalMailRequest) *capture.InternalMailPatch {
	if req == nil {
		return nil
	}
	selections := make([]capture.InternalMailSelection, len(req.Sources))
	for i, source := range req.Sources {
		selections[i] = capture.InternalMailSelection{ConnectionID: ids.UUID(source.ConnectionId), Containers: source.Containers}
		if source.HistorySince != nil {
			selections[i].HistorySince = new(source.HistorySince.Time)
		}
	}
	return &capture.InternalMailPatch{Enabled: req.Enabled, Sources: selections}
}

func contractInternalMailSettings(s *capture.InternalMailSettings) *crmcontracts.InternalMailSettings {
	if s == nil {
		return nil
	}
	out := &crmcontracts.InternalMailSettings{
		Enabled: s.Policy.Enabled, Since: s.Policy.Since,
		Sources:          make([]crmcontracts.InternalMailSource, len(s.Policy.Sources)),
		AvailableSources: make([]crmcontracts.InternalMailSourceOption, len(s.AvailableSources)),
	}
	for i, source := range s.Policy.Sources {
		out.Sources[i] = crmcontracts.InternalMailSource{
			ConnectionId: openapi_types.UUID(source.ConnectionID), Containers: source.Containers, OwnerUserId: openapi_types.UUID(source.OwnerUserID),
			Provider: source.Provider, Generation: source.Generation, EnrolledAt: source.EnrolledAt, EnrolledBy: source.EnrolledBy,
		}
	}
	for i, source := range s.Policy.Sources {
		if source.HistorySince != nil {
			out.Sources[i].HistorySince = &openapi_types.Date{Time: *source.HistorySince}
		}
	}
	for i, source := range s.AvailableSources {
		out.AvailableSources[i] = crmcontracts.InternalMailSourceOption{
			ConnectionId: openapi_types.UUID(source.ConnectionID), OwnerUserId: openapi_types.UUID(source.OwnerUserID), Provider: source.Provider,
			Generation: source.Generation, AccountLabel: source.AccountLabel, OwnerName: source.OwnerName,
		}
	}
	return out
}
