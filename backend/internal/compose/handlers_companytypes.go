// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The company type directory (GET /v1/company-types): the types installed
// extensions add beside core's own, with the label a screen shows.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/companytypes"
)

// ListCompanyTypes (GET /v1/company-types) is served beside the transport
// directory, whose receiver it borrows: both are stateless process-level
// directories. Any signed-in seat may read it, because a rep's company list shows
// and filters on these types. Values and labels are declared in code, never typed
// by an administrator, so no workspace data is disclosed.
func (channelProvidersHandlers) ListCompanyTypes(w http.ResponseWriter, r *http.Request) {
	declared := companytypes.All()
	data := make([]crmcontracts.CompanyTypeOption, 0, len(declared))
	for _, d := range declared {
		data = append(data, crmcontracts.CompanyTypeOption{Value: d.Value, Label: d.Label, Unit: string(d.Unit)})
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.CompanyTypeDirectory{Data: data})
}
