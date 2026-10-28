// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// PreviewControllerMail serves GET /contacts/{id}/consent/mail-preview.
func (h Handlers) PreviewControllerMail(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id, _ crmcontracts.PreviewControllerMailParams) {
	httperr.NotImplemented(w, r, "PreviewControllerMail")
}
