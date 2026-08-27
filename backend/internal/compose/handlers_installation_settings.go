// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation-settings surface (ADR-0090/A135): read the organization's
// name, reporting zone and base currency (every role), change them (admin/ops,
// human-only). Thin transport — the identity store owns the RBAC gate, the
// per-setting validation, the base-currency freeze and the audit-only write.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

type installationSettingsHandlers struct {
	store *identity.InstallationSettingsStore
	// maxUploadBytes is the deployment's attachment ceiling (OPS-CFG-12),
	// published so an upload surface enforces the number THIS installation
	// applies rather than one compiled into the client.
	//
	// It rides this read rather than a read of its own because it answers the
	// same question the rest of the response does — what is true of this
	// installation — and because every role may read it: whoever can upload a
	// file can be told how large a file is worth sending.
	maxUploadBytes int64
}

func (h installationSettingsHandlers) GetInstallationSettings(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetInstallationSettings")
		return
	}
	s, err := h.store.GetInstallation(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, h.toContract(s))
}

func (h installationSettingsHandlers) UpdateInstallationSettings(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "UpdateInstallationSettings")
		return
	}
	// Human-only (x-agent-access): an agent never renames the organization or
	// re-bases its reporting currency. The store re-checks the admin/ops grant.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.UpdateInstallationSettingsRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	patch := identity.InstallationPatch{
		Name:         req.Name,
		Timezone:     req.Timezone,
		BaseCurrency: req.BaseCurrency,
	}
	if req.BaseLanguage != nil {
		// The generated enum refuses an unknown value at the edge, so a code
		// that reaches here is one the contract admits. The entry validates it
		// again on the write — the contract and the setting each state the
		// language set, and neither defers to the other.
		if !req.BaseLanguage.Valid() {
			httperr.Write(w, r, httperr.Validation("base_language", "invalid", "a base language is one of en, de, vi"))
			return
		}
		lang := string(*req.BaseLanguage)
		patch.BaseLanguage = &lang
	}
	// No range check here, unlike BaseLanguage above. That one exists because
	// the generated enum type carries a Valid() method worth calling; this
	// field has no such type, and the entry's own refusal is already the better
	// answer — settings.InvalidValue implements apperrors.FieldFault, so an
	// out-of-range month comes back as a 422 naming the setting and carrying
	// identity's own sentence, which quotes the value that was refused. A
	// second check here would name a different field and say less.
	patch.FiscalYearStartMonth = req.FiscalYearStartMonth
	s, err := h.store.UpdateInstallation(r.Context(), patch)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, h.toContract(s))
}

// toContract maps the stored values onto the wire shape, and adds the one field
// that is not stored at all: the upload ceiling is a deployment fact, so it
// arrives from composition rather than from the settings table — the same way
// base_currency_locked is derived rather than kept.
//
// The lock reason is omitted rather than sent empty when the currency is still
// changeable: an empty string would render as a reason that says nothing.
func (h installationSettingsHandlers) toContract(s identity.InstallationSettings) crmcontracts.InstallationSettings {
	out := crmcontracts.InstallationSettings{
		Name:                 s.Name,
		Timezone:             s.Timezone,
		BaseCurrency:         s.BaseCurrency,
		BaseLanguage:         crmcontracts.InstallationSettingsBaseLanguage(s.BaseLanguage),
		FiscalYearStartMonth: s.FiscalYearStartMonth,
		BaseCurrencyLocked:   s.BaseCurrencyLocked,
		MaxUploadBytes:       h.maxUploadBytes,
	}
	if s.BaseCurrencyLockedReason != "" {
		reason := s.BaseCurrencyLockedReason
		out.BaseCurrencyLockedReason = &reason
	}
	return out
}
