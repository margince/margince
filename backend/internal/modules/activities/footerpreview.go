// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MailFooterPreview describes owed disclosures without minting recipient tokens.
type MailFooterPreview struct {
	Disclosures       []DisclosureLine
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
	owed, err := s.requiredDisclosures(ctx, string(surface.disclosureCategory()), in.Recipients, in.SendingEntity)
	if err != nil {
		return MailFooterPreview{}, err
	}
	return MailFooterPreview{Disclosures: owed, OffersUnsubscribe: surface.carries()}, nil
}

func (s *Store) requiredDisclosures(ctx context.Context, category string, recipients []string, entity ids.UUID) ([]DisclosureLine, error) {
	owed, err := s.disclosuresFor(ctx, category, recipients, entity)
	if err != nil {
		return nil, err
	}
	if kinds := unmeetable(owed); len(kinds) > 0 {
		return nil, &UndisclosableError{Kinds: kinds}
	}
	return owed, nil
}
