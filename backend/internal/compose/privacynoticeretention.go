// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// privacyNoticeHeld hands the privacy page what the privacy module reads about
// a subject. The floor it applies is the installation's own.
type privacyNoticeHeld struct{ db *database.DB }

func (r privacyNoticeHeld) NoticeHeld(ctx context.Context, subject consent.NoticeSubject) (consent.NoticeHeld, error) {
	read, err := privacy.AssembleNoticeDisclosure(ctx, r.db, identity.CountryForDecision,
		privacy.NoticeSubject{ContactID: subject.ContactID, LeadID: subject.LeadID})
	if err != nil {
		return consent.NoticeHeld{}, err
	}
	out := consent.NoticeHeld{Categories: read.Categories}
	for _, use := range read.Uses {
		out.Uses = append(out.Uses, consent.NoticeUse{PurposeKey: use.PurposeKey, Basis: use.Basis})
	}
	for _, rule := range read.Retention {
		out.Retention = append(out.Retention, consent.RetentionRule{
			Scope: rule.Scope, RetainDays: rule.RetainDays, Action: rule.Action, KeptByLaw: lawFloor(rule.KeptByLaw),
		})
	}
	return out, nil
}

func lawFloor(class *jurisdiction.RetentionClass) *consent.LawFloor {
	if class == nil {
		return nil
	}
	return &consent.LawFloor{
		Name: string(class.Name), Keep: class.Keep.String(),
		FromYearEnd: class.Anchor == jurisdiction.AnchorCalendarYearEnd,
	}
}
