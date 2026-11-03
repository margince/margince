// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
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
// matching send would stage now. It runs the send's guards and keeps nothing.
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
		var err error
		if out, err = s.previewControllerMailTx(ctx, tx, contactID, kind, language); err != nil {
			return err
		}
		return errMailPreviewDone
	})
	if errors.Is(err, errMailPreviewDone) {
		return out, nil
	}
	return ControllerMailPreview{}, err
}

// errMailPreviewDone rolls the preview's transaction back once it has its answer.
var errMailPreviewDone = errors.New("consent: mail preview complete")

func (s *Store) previewControllerMailTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, kind, language string) (ControllerMailPreview, error) {
	to, err := admitLinkTx(ctx, tx, linkRequest{contactID: contactID, preview: true})
	if err != nil {
		return ControllerMailPreview{}, err
	}
	if !s.canSendConfirm() {
		return ControllerMailPreview{}, ErrNoMailLane
	}
	issued := s.now().UTC()
	in := confirmMailInput{
		contactID: contactID, recipient: to, kind: kind,
		language: s.sendLanguage(ctx, tx, language), expiresAt: issued.Add(confirmTokenTTL),
	}
	// The engine allows a confirmation only on a live link of its kind, so the
	// preview writes one, digest only, which the rollback above undoes.
	if _, err := insertContactLinkTx(ctx, tx, contactLink{
		contactID: contactID, tokenHash: hashPublicToken(ids.NewV7().String()), deliveredTo: to,
		issued: issued, expires: in.expiresAt, kind: kind, language: in.language,
	}); err != nil {
		return ControllerMailPreview{}, err
	}
	rendered, category, err := s.renderConfirmMail(ctx, tx, in)
	if err != nil {
		return ControllerMailPreview{}, err
	}
	presented, err := s.confirmSender.PresentConfirmationTx(ctx, tx, ConfirmationSend{
		ContactID: contactID, Recipient: to, Category: category, ExpiresAt: in.expiresAt, Rendered: rendered,
	})
	if err != nil {
		return ControllerMailPreview{}, err
	}
	presented.Text = strings.ReplaceAll(presented.Text, linkPlaceholder, mailcopy.For(rendered.Language).PreviewLinkMarker)
	return ControllerMailPreview{
		Kind:            kind,
		Language:        rendered.Language,
		DefaultLanguage: string(mailcopy.Carried(s.mailLanguage(ctx, tx))),
		PresentedMail:   presented,
		To:              to,
	}, nil
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
