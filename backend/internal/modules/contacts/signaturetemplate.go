// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// GetSignatureTemplate reads the workspace's signature layout through the
// settings gate, so only a seat that may read installation settings sees it.
func (s *Store) GetSignatureTemplate(ctx context.Context) (string, error) {
	if s.settings == nil {
		return "", fmt.Errorf("contacts: the signature template is not wired; the installation cannot read it")
	}
	return settings.Get(ctx, s.settings, SignatureTemplate)
}

// SaveSignatureTemplate writes the layout through the settings store, which
// validates, gates and audits it. The send path sanitizes the markup, so what
// an admin types never reaches a recipient unchecked.
func (s *Store) SaveSignatureTemplate(ctx context.Context, template string) (string, error) {
	if s.settings == nil {
		return "", fmt.Errorf("contacts: the signature template is not wired; the installation cannot change it")
	}
	trimmed := strings.TrimSpace(template)
	if err := settings.Set(ctx, s.settings, SignatureTemplate, trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

// GetEmailSignatureTemplate implements GET /email-signature-template.
func (h Handlers) GetEmailSignatureTemplate(w http.ResponseWriter, r *http.Request) {
	template, err := h.store.GetSignatureTemplate(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.EmailSignatureTemplate{Template: template})
}

// SaveEmailSignatureTemplate implements PUT /email-signature-template (admin/ops, human only).
func (h Handlers) SaveEmailSignatureTemplate(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.EmailSignatureTemplate
	if !httperr.Decode(w, r, &body) {
		return
	}
	template, err := h.store.SaveSignatureTemplate(r.Context(), body.Template)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.EmailSignatureTemplate{Template: template})
}
