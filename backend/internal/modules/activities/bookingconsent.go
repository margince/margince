// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PrepareBookingConsent validates and resolves consent without recording it or sending mail.
func PrepareBookingConsent(ctx context.Context, in crmcontracts.CaptureConsent, links []ActivityLinkInput, capturer ConsentCapturer) (ids.UUID, BookingConsent, error) {
	if capturer == nil {
		return ids.Nil, BookingConsent{}, apperrors.ErrPermissionDenied
	}
	if in.PolicyVersion == "" {
		return ids.Nil, BookingConsent{}, httperr.Validation("consent.policy_version", codeRequired, "the consent wording version shown to the subject is required")
	}
	if in.Wording == nil || strings.TrimSpace(*in.Wording) == "" {
		return ids.Nil, BookingConsent{}, httperr.Validation("consent.wording", codeRequired, "the consent wording shown to the subject is required")
	}
	contact, err := BookingConsentSubject(links)
	if err != nil {
		return ids.Nil, BookingConsent{}, err
	}
	purpose, err := capturer.ScopedPurpose(ctx, requestedPurpose(in))
	if err != nil {
		return ids.Nil, BookingConsent{}, err
	}
	marketing, err := prepareBookingMarketing(ctx, in, "", capturer)
	if err != nil {
		return ids.Nil, BookingConsent{}, err
	}
	return contact, BookingConsent{PurposeID: purpose, PolicyVersion: in.PolicyVersion, Wording: *in.Wording, Marketing: marketing}, nil
}

func prepareBookingMarketing(ctx context.Context, in crmcontracts.CaptureConsent, tickedFrom string, capturer ConsentCapturer) (*BookingMarketing, error) {
	m := in.Marketing
	if m == nil {
		return nil, nil
	}
	if strings.TrimSpace(m.PolicyVersion) == "" {
		return nil, httperr.Validation("consent.marketing.policy_version", codeRequired, "the marketing wording version shown to the subject is required")
	}
	if strings.TrimSpace(m.Wording) == "" {
		return nil, httperr.Validation("consent.marketing.wording", codeRequired, "the marketing wording shown to the subject is required")
	}
	purpose := ids.UUID(m.PurposeId)
	if err := capturer.ValidateMarketingPurpose(ctx, purpose); err != nil {
		return nil, err
	}
	return &BookingMarketing{PurposeID: purpose, PolicyVersion: m.PolicyVersion, Wording: m.Wording, TickedFrom: tickedFrom}, nil
}
