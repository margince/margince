// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (r approvalPreviewReader) appendMailSignature(ctx context.Context, approval crmcontracts.Approval, in activities.SendEmailInput, out *crmcontracts.ApprovalPreview) error {
	human, humanSend := strings.CutPrefix(approval.ProposedBy, "human:")
	if !humanSend {
		if strings.HasPrefix(approval.ProposedBy, "agent:") || strings.HasPrefix(approval.ProposedBy, "system:") || strings.HasPrefix(approval.ProposedBy, "connector:") {
			*out.Effects = append(*out.Effects, "send_without_human_signature")
			return nil
		}
		return apperrors.ErrInvalidArgument
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.String() != human {
		return r.privateSignatureConflict(ctx, out)
	}
	var language string
	if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		language, err = identity.BaseLanguageOf(ctx, tx)
		return err
	}); err != nil {
		return err
	}
	store := r.activities.WithSignature(signatureReader{store: contacts.NewStore(r.db)}).
		WithSenderName(identity.NewServiceFor(r.db)).
		WithBaseLanguage(activities.BaseLanguageFunc(func(context.Context) string { return language }))
	sign, err := store.PreviewMailSignOff(ctx, in.Body, in.Subject)
	if err != nil {
		return err
	}
	if sign.Text != "" {
		*out.Details = append(*out.Details, previewDetail("signature_text", sign.Text))
	}
	if sign.HTML != "" {
		*out.Details = append(*out.Details, previewDetail("signature_html", sign.HTML))
	}
	return nil
}

func (r approvalPreviewReader) appendMailPresentation(ctx context.Context, approval crmcontracts.Approval, in activities.SendEmailInput, out *crmcontracts.ApprovalPreview) error {
	if err := r.appendMailSignature(ctx, approval, in, out); err != nil {
		return err
	}
	return r.appendMailFooter(ctx, in, out)
}

func (r approvalPreviewReader) privateSignatureConflict(ctx context.Context, out *crmcontracts.ApprovalPreview) error {
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		said := approvalSummaryCopyIn(ctx, tx)
		out.Conflicts = &[]crmcontracts.ApprovalPreviewConflict{{Kind: "sender_signature_private", Summary: said.signaturePrivate, BlocksApproval: true}}
		return nil
	})
	if err != nil {
		return err
	}
	return apperrors.ErrPermissionDenied
}
