// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// footerKindUnsubscribeLink names a send that owed a stop link and could be
// given none.
const footerKindUnsubscribeLink = "unsubscribe_link"

func (s *Store) footerFindings(lines []DisclosureLine, recipients []string, advertising bool) []commsauthz.FooterFinding {
	findings := unmeetable(lines)
	if !advertising || s.unsubscribe == nil {
		return findings
	}
	if distinctAddresses(recipients) > 1 {
		findings = append(findings, commsauthz.FooterFinding{Kind: "recipient_private_link"})
	}
	if s.publicOriginUsable() != nil {
		findings = append(findings, commsauthz.FooterFinding{Kind: "public_opt_out_link"})
	}
	return findings
}

func (s *Store) assessPreviewFooter(ctx context.Context, in PreviewSendInput, set commsauthz.DecisionSet) (commsauthz.DecisionSet, error) {
	if s.disclosures == nil && s.unsubscribe == nil {
		return set, nil
	}
	entity, err := s.sendingEntity(ctx)
	if err != nil {
		return set, err
	}
	surface := surfaceFor(in.Context, in.MarketingPurpose, in.LegacyPurposeKey)
	if set.AdvertisesToASubject() {
		surface.category = commsauthz.CategoryMarketing
	}
	lang := s.footerLanguage(ctx, in.Body, in.Subject)
	lines, err := s.disclosuresFor(ctx, string(surface.disclosureCategory()), in.Recipients, in.Evidence.RecipientCountries, entity, string(lang))
	if err != nil {
		return set, err
	}
	findings := s.footerFindings(lines, in.Recipients, surface.privateLinks())
	for i, decision := range set.Decisions {
		set.Decisions[i] = decision.WithFooterFindings(findings)
	}
	return set, nil
}
