// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The transport for a legal entity's logo. It takes the company logo's decode,
// storage and collection unchanged, so an entity's mark is refused and stored
// by the rules the company's is.

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UploadLegalEntityLogo implements POST /legal-entities/{id}/logo.
func (h companyHandlers) UploadLegalEntityLogo(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UploadLegalEntityLogoParams) {
	if h.store == nil || h.blob == nil {
		httperr.NotImplemented(w, r, "uploadLegalEntityLogo")
		return
	}
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	entityID := ids.From[ids.LegalEntityKind](ids.UUID(id))
	// Every refusal the write would give comes before the upload is read. So a
	// caller who may not change the logo leaves no bytes and no intent behind.
	if err := h.store.EnsureLegalEntityLogoWritable(r.Context(), entityID, ifVersion); err != nil {
		httperr.Write(w, r, err)
		return
	}
	png, _, ok := decodeCompanyLogoUpload(w, r)
	if !ok {
		return
	}
	workspace, found := principal.WorkspaceID(r.Context())
	if !found {
		httperr.Write(w, r, errors.New("compose: a legal entity logo upload outside a workspace context"))
		return
	}
	markKey := legalEntityLogoKey(ids.From[ids.WorkspaceKind](workspace), entityID)
	if err := h.store.RecordLogoIntent(r.Context(), markKey); err != nil {
		httperr.Write(w, r, err)
		return
	}
	key, err := contacts.PutLogo(r.Context(), h.blob, markKey, png)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// A refused write leaves the bytes for the ledger, for the reason
	// uploadCompanyMark gives.
	superseded, err := h.store.SetLegalEntityLogo(r.Context(), entityID, key, ifVersion)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.collectLogoObject(r.Context(), superseded)
	h.writeLegalEntity(w, r, entityID)
}

// DeleteLegalEntityLogo implements DELETE /legal-entities/{id}/logo.
func (h companyHandlers) DeleteLegalEntityLogo(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.DeleteLegalEntityLogoParams) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "deleteLegalEntityLogo")
		return
	}
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	entityID := ids.From[ids.LegalEntityKind](ids.UUID(id))
	removed, err := h.store.ClearLegalEntityLogo(r.Context(), entityID, ifVersion)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.collectLogoObject(r.Context(), removed)
	h.writeLegalEntity(w, r, entityID)
}

func (h companyHandlers) writeLegalEntity(w http.ResponseWriter, r *http.Request, id ids.LegalEntityID) {
	entity, err := h.store.GetLegalEntity(r.Context(), id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, contacts.LegalEntityWire(entity))
}

// legalEntityLogoKey mints a key per upload, under the company logo's kind so
// the one logo sweep owns it, for the reason companyLogoKey mints per attempt.
func legalEntityLogoKey(wsID ids.WorkspaceID, id ids.LegalEntityID) string {
	return blobstore.WorkspaceKey(wsID, companyLogoKind, "legal-entity/"+id.String()+"/"+ids.NewV7().String())
}
