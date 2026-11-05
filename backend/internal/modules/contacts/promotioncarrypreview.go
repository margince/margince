// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// LeadPromotionCarry collects dependent records that must move with a promotion.
type LeadPromotionCarry struct {
	Activities []LeadPromotionActivity
	// WithheldActivities counts the lead's activities that move too but that
	// the reader may not read, so the preview names how many without them.
	WithheldActivities int
	Consent            []ConsentCarryChange
	Stops              []commsauthz.CarriedStop
	ConsentLinks       commsauthz.CarriedConsentLinks
}

type stopPreviewer interface {
	PreviewStopsTx(context.Context, pgx.Tx, commsauthz.StopSubject, commsauthz.StopSubject) ([]commsauthz.CarriedStop, error)
}

type consentLinksPreviewer interface {
	PreviewConsentLinksTx(context.Context, pgx.Tx, commsauthz.StopSubject, commsauthz.StopSubject) (commsauthz.CarriedConsentLinks, error)
}

func (s *Store) previewCarriedStops(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) ([]commsauthz.CarriedStop, error) {
	if reader, ok := s.stopCarrier.(stopPreviewer); ok {
		return reader.PreviewStopsTx(ctx, tx, from, to)
	}
	held, err := holdsALiveStop(ctx, tx, from)
	if err != nil {
		return nil, err
	}
	if held {
		return nil, &StopCarrierNotWiredError{}
	}
	return []commsauthz.CarriedStop{}, nil
}

func (s *Store) previewCarriedConsentLinks(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) (commsauthz.CarriedConsentLinks, error) {
	if reader, ok := s.consentCarrier.(consentLinksPreviewer); ok {
		return reader.PreviewConsentLinksTx(ctx, tx, from, to)
	}
	held, err := holdsAConsentSatellite(ctx, tx, from)
	if err != nil {
		return commsauthz.CarriedConsentLinks{}, err
	}
	if held {
		return commsauthz.CarriedConsentLinks{}, &SatelliteCarrierNotWiredError{Holding: "consent links or recorded lawful bases"}
	}
	return commsauthz.CarriedConsentLinks{}, nil
}

func (s *Store) previewPromotionCarry(ctx context.Context, tx pgx.Tx, lead ids.LeadID, contact ids.ContactID, out *LeadPromotionCarry) error {
	var err error
	out.Activities, out.WithheldActivities, err = previewPromotionActivities(ctx, tx, lead, contact)
	if err != nil {
		return err
	}
	out.Consent, err = previewPromotionConsent(ctx, tx, lead, contact)
	if err != nil {
		return err
	}
	out.Stops, err = s.previewCarriedStops(ctx, tx, commsauthz.LeadStopSubject(lead), commsauthz.ContactStopSubject(contact))
	if err != nil {
		return err
	}
	out.ConsentLinks, err = s.previewCarriedConsentLinks(ctx, tx, commsauthz.LeadStopSubject(lead), commsauthz.ContactStopSubject(contact))
	return err
}

type overridePreviewer interface {
	PreviewOverridesTx(context.Context, pgx.Tx, commsauthz.StopSubject, commsauthz.StopSubject) ([]commsauthz.CarriedOverride, error)
}

func (s *Store) previewCarriedOverrides(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) ([]commsauthz.CarriedOverride, error) {
	if reader, ok := s.stopCarrier.(overridePreviewer); ok {
		return reader.PreviewOverridesTx(ctx, tx, from, to)
	}
	held, err := holdsALiveOverride(ctx, tx, from)
	if err != nil {
		return nil, err
	}
	if held {
		return nil, &OverrideCarrierNotWiredError{}
	}
	return nil, nil
}

type preferenceLinksPreviewer interface {
	PreviewPreferenceLinksTx(context.Context, pgx.Tx, ids.ContactID, ids.ContactID, []ids.UUID) ([]commsauthz.CarriedPreferenceLink, error)
}

func (s *Store) previewMergePreferenceLinks(ctx context.Context, tx pgx.Tx, source, target ids.UUID, addresses []MergeAddress) ([]commsauthz.CarriedPreferenceLink, error) {
	from, to := ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target)
	if reader, ok := s.consentCarrier.(preferenceLinksPreviewer); ok {
		moving := []ids.UUID{}
		for _, address := range addresses {
			if address.Kind == fieldEmail {
				moving = append(moving, address.ID)
			}
		}
		return reader.PreviewPreferenceLinksTx(ctx, tx, from, to, moving)
	}
	held, err := holdsAConsentSatellite(ctx, tx, commsauthz.ContactStopSubject(from))
	if err != nil {
		return nil, err
	}
	if held {
		return nil, &SatelliteCarrierNotWiredError{Holding: "preference links"}
	}
	return nil, nil
}
