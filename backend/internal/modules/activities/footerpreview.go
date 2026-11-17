// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// MailFooterPreview describes owed disclosures without minting recipient tokens.
type MailFooterPreview struct {
	Disclosures       []DisclosureLine
	Findings          []commsauthz.FooterFinding
	OffersUnsubscribe bool
}

// PreviewMailFooter never calls the linker that creates recipient tokens. It
// takes the category the send will use: the caller's claim, widened to
// marketing when the engine reads a recipient as advertising.
func (s *Store) PreviewMailFooter(ctx context.Context, origin SendOrigin, links []ActivityLinkInput, in SendEmailInput, gate ConsentGate) (MailFooterPreview, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return MailFooterPreview{}, err
	}
	surface, err := surfaceForRecipients(ctx, in, origin, links, gate)
	if err != nil {
		return MailFooterPreview{}, err
	}
	lang := s.footerLanguage(ctx, in.Body, in.Subject)
	owed, err := s.disclosuresFor(ctx, string(surface.disclosureCategory()), in.Recipients, in.SendingEntity, string(lang))
	if err != nil {
		return MailFooterPreview{}, err
	}
	findings := s.footerFindings(owed, in.Recipients, surface.privateLinks())
	return MailFooterPreview{Disclosures: owed, Findings: findings, OffersUnsubscribe: surface.privateLinks() && distinctAddresses(in.Recipients) == 1 && s.publicOriginUsable() == nil}, nil
}
