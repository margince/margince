// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"cmp"
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PresentedMail is a controller mail as the relay sends it.
type PresentedMail struct {
	From    string
	Subject string
	Text    string
}

// ControllerMailPreview is the mail a controller send would stage now.
type ControllerMailPreview struct {
	Kind            string
	Language        string
	DefaultLanguage string
	PresentedMail
	To string
}

// PreviewControllerMail serves GET /contacts/{id}/consent/mail-preview.
func (h Handlers) PreviewControllerMail(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.PreviewControllerMailParams) {
	language := ""
	if params.Language != nil {
		language = string(*params.Language)
	}
	preview, err := h.store.PreviewControllerMail(r.Context(), pathID[ids.ContactKind](id), string(params.Kind), language)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireControllerMailPreview(preview))
}

// PreviewControllerMail renders the record confirmation or privacy notice the
// matching send would stage now. It runs the send's guards and mints no link.
func (s *Store) PreviewControllerMail(ctx context.Context, contactID ids.ContactID, kind, language string) (ControllerMailPreview, error) {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return ControllerMailPreview{}, err
	}
	if kind != LinkRecordConfirmation && kind != LinkPrivacyNotice {
		return ControllerMailPreview{}, &ValidationError{Field: fieldKind, Reason: "preview a record confirmation or a privacy notice"}
	}
	if err := requireMailLanguage(language); err != nil {
		return ControllerMailPreview{}, err
	}
	var out ControllerMailPreview
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		to, err := admitLinkTx(ctx, tx, linkRequest{contactID: contactID})
		if err != nil {
			return err
		}
		if !s.canSendConfirm() {
			return ErrNoMailLane
		}
		installation := s.mailLanguage(ctx, tx)
		rendered, _, err := s.renderConfirmMail(ctx, tx, confirmMailInput{
			kind:      kind,
			language:  cmp.Or(language, installation),
			expiresAt: s.now().UTC().Add(confirmTokenTTL),
		})
		if err != nil {
			return err
		}
		presented, err := s.confirmSender.PresentConfirmationTx(ctx, tx, rendered)
		if err != nil {
			return err
		}
		presented.Text = strings.ReplaceAll(presented.Text, linkPlaceholder, mailcopy.For(rendered.Language).PreviewLinkMarker)
		out = ControllerMailPreview{
			Kind:            kind,
			Language:        rendered.Language,
			DefaultLanguage: knownMailLanguage(installation),
			PresentedMail:   presented,
			To:              to,
		}
		return nil
	})
	return out, err
}

func knownMailLanguage(language string) string {
	if mailcopy.Known(language) {
		return language
	}
	return string(mailcopy.Fallback)
}

func wireControllerMailPreview(p ControllerMailPreview) crmcontracts.ControllerMailPreview {
	languages := make([]crmcontracts.MailLanguage, 0, len(mailcopy.Languages()))
	for _, language := range mailcopy.Languages() {
		languages = append(languages, crmcontracts.MailLanguage(language))
	}
	out := crmcontracts.ControllerMailPreview{
		Kind:            crmcontracts.ControllerMailPreviewKind(p.Kind),
		Language:        crmcontracts.MailLanguage(p.Language),
		DefaultLanguage: crmcontracts.MailLanguage(p.DefaultLanguage),
		Languages:       languages,
		ToAddress:       p.To,
		Subject:         p.Subject,
		Text:            p.Text,
	}
	if p.From != "" {
		out.FromAddress = &p.From
	}
	return out
}

// sendLanguageOf reads the optional body of a controller send; no body picks
// no language.
func sendLanguageOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.ContentLength == 0 {
		return "", true
	}
	var body crmcontracts.ControllerMailSend
	if !httperr.Decode(w, r, &body) {
		return "", false
	}
	if body.Language == nil {
		return "", true
	}
	return string(*body.Language), true
}
