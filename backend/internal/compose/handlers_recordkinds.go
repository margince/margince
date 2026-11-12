// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The record kind directory (GET /v1/record-kinds): the kinds installed
// extensions add beside core's own, as far as the caller may read them.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// recordKindActions are the four actions a kind's rights are held in, in the
// order the directory lists them.
var recordKindActions = []principal.Action{
	principal.ActionCreate, principal.ActionRead, principal.ActionUpdate, principal.ActionDelete,
}

// ListRecordKinds (GET /v1/record-kinds) is served beside the transport
// directory, whose receiver it borrows. Both are process-level directories. A
// kind the seat holds no read right on is left out. Every right on an extension
// kind is denied until a role is granted it. A screen has nothing to draw for
// a kind its reader cannot open.
func (channelProvidersHandlers) ListRecordKinds(w http.ResponseWriter, r *http.Request) {
	declared := recordkinds.All()
	data := make([]crmcontracts.RecordKindOption, 0, len(declared))
	for _, d := range declared {
		if !auth.Allows(r.Context(), d.Name, principal.ActionRead) {
			continue
		}
		fields := make([]crmcontracts.RecordKindField, 0, len(d.Fields))
		for _, f := range d.Fields {
			fields = append(fields, crmcontracts.RecordKindField{Name: f.Name, Label: f.Label})
		}
		held := make([]crmcontracts.RecordKindOptionActions, 0, len(recordKindActions))
		for _, a := range recordKindActions {
			if auth.Allows(r.Context(), d.Name, a) {
				held = append(held, crmcontracts.RecordKindOptionActions(a))
			}
		}
		data = append(data, crmcontracts.RecordKindOption{
			Name: d.Name, Unit: string(d.Unit), Kind: d.Kind, DisplayName: d.DisplayName,
			Fields: fields, NaturalKey: d.NaturalKey,
			DeletePolicy: crmcontracts.RecordKindOptionDeletePolicy(d.DeletePolicy), Actions: held,
		})
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.RecordKindDirectory{Data: data})
}
