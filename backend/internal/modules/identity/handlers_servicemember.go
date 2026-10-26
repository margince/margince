// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// serviceExtensionMax is the contract's own bound on `extension`, enforced here
// because the generated wrapper enforces no maxLength.
const serviceExtensionMax = 64

// DeclareServiceMember (POST /users/service-members) creates the member an
// extension's unattended feed runs as.
func (h Handlers) DeclareServiceMember(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req crmcontracts.ServiceMemberRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	extension := strings.TrimSpace(req.Extension)
	if !values.HasVisibleText(extension) || utf8.RuneCountInString(extension) > serviceExtensionMax {
		httperr.Write(w, r, httperr.Validation("extension", "length", "an extension name of 1–64 characters is required"))
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if !values.HasVisibleText(name) || utf8.RuneCountInString(name) > 255 {
		httperr.Write(w, r, httperr.Validation("display_name", "length", "a display name of 1–255 characters is required"))
		return
	}
	member, err := h.svc.DeclareServiceMember(r.Context(), actor, ServiceMemberInput{Extension: extension, DisplayName: name})
	if err != nil {
		httperr.Write(w, r, companyNotDescribedRefusal(err))
		return
	}
	h.writeUserByID(w, r, actor, member, http.StatusCreated)
}

// ArchiveServiceMember (POST /users/service-members/{id}/archive) retires one.
func (h Handlers) ArchiveServiceMember(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	if err := h.svc.ArchiveServiceMember(r.Context(), actor, ids.UserID{UUID: ids.UUID(id)}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
