// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// What a project search may be narrowed by, on the tool surface.
//
// A filter this set has no binding for is an ERROR rather than a dropped
// clause: a caller who narrowed a search and got every row back has been told
// something false about what they are looking at.

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The filter names, spelled once so a binding and the caller's key cannot
// drift apart.
const (
	filterKey            = "key"
	filterOrganizationID = "organization_id"
	filterOwnerID        = "owner_id"
	filterPhase          = "phase"
)

var projectListFilters = storekit.FilterSet[ListProjectsInput]{
	filterKey: storekit.FilterWord(func(in *ListProjectsInput, v *string) { in.Key = v }),
	filterOrganizationID: storekit.FilterID(
		func(in *ListProjectsInput, id *ids.OrganizationID) { in.OrganizationID = id }),
	filterOwnerID: storekit.FilterID(func(in *ListProjectsInput, id *ids.UserID) { in.OwnerID = id }),
	filterPhase:   storekit.FilterWord(func(in *ListProjectsInput, v *string) { in.Phase = v }),
}
