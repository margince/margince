// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type MailFooterPreview struct {
	Disclosures       []DisclosureLine
	OffersUnsubscribe bool
}

// PreviewMailFooter never calls the linker that creates recipient tokens.
func (s *Store) PreviewMailFooter(ctx context.Context, in SendEmailInput) (MailFooterPreview, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return MailFooterPreview{}, err
	}
	surface := surfaceFor(in.Context, in.MarketingPurpose, in.ConsentPurpose)
	owed, err := s.requiredDisclosures(ctx, string(surface.disclosureCategory()))
	if err != nil {
		return MailFooterPreview{}, err
	}
	return MailFooterPreview{Disclosures: owed, OffersUnsubscribe: surface.carries()}, nil
}

func (s *Store) requiredDisclosures(ctx context.Context, category string) ([]DisclosureLine, error) {
	owed, err := s.disclosuresFor(ctx, category)
	if err != nil {
		return nil, err
	}
	if kinds := unmeetable(owed); len(kinds) > 0 {
		return nil, &UndisclosableError{Kinds: kinds}
	}
	return owed, nil
}
