// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

func (s *Service) noticeDutiesSection(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, out *crmcontracts.Contact360) error {
	duties, err := s.consent.OpenNoticeCasesDueSoonestTx(ctx, tx, consent.NoticeAgendaInput{ContactID: &contactID, Limit: sectionCap})
	if err != nil {
		return err
	}
	wire := make([]crmcontracts.ContactNoticeDuty, 0, len(duties))
	for _, duty := range duties {
		wire = append(wire, crmcontracts.ContactNoticeDuty{Id: openapi_types.UUID(duty.ID), Rule: crmcontracts.ContactNoticeDutyRule(duty.Rule), DueAt: duty.DueAt, Blocked: duty.Blocked, DeadlineIsUpperBound: &duty.DeadlineIsUpperBound})
	}
	reviews, err := s.consent.NoticeSourceReviewsTx(ctx, tx, contactID)
	if err != nil {
		return err
	}
	reviewIDs := make([]openapi_types.UUID, 0, len(reviews))
	for _, id := range reviews {
		reviewIDs = append(reviewIDs, openapi_types.UUID(id))
	}
	out.PrivacySourceReviews = &reviewIDs
	out.PrivacyNoticeDuties = &wire
	return nil
}

func (said momentCopy) noticeDutyMoment(now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	if page.PrivacyNoticeDuties == nil || len(*page.PrivacyNoticeDuties) == 0 {
		return crmcontracts.ContactMoment{}, false
	}
	duty := (*page.PrivacyNoticeDuties)[0]
	title := said.Say(langcopy.Phrase{En: "Privacy notice owed", De: "Datenschutzhinweis ausstehend", Vi: "Cần gửi thông báo quyền riêng tư"})
	reason := fmt.Sprintf(said.Say(langcopy.Phrase{En: "A disclosure duty is due by %s.", De: "Die Informationspflicht ist bis %s zu erfüllen.", Vi: "Nghĩa vụ thông báo đến hạn vào %s."}), duty.DueAt.In(said.zone).Format(time.DateOnly))
	if duty.DeadlineIsUpperBound != nil && *duty.DeadlineIsUpperBound {
		reason = fmt.Sprintf(said.Say(langcopy.Phrase{En: "A notice is owed. The acquisition date is unknown: send by %s at the latest; it may already be due.", De: "Ein Datenschutzhinweis ist ausstehend. Das Erfassungsdatum ist unbekannt: spätestens bis %s senden; er kann bereits fällig sein.", Vi: "Cần gửi thông báo. Chưa rõ ngày nhận thông tin: gửi chậm nhất vào %s; nghĩa vụ có thể đã đến hạn."}), duty.DueAt.In(said.zone).Format(time.DateOnly))
	}
	if duty.Blocked {
		reason = said.Say(langcopy.Phrase{En: "Notice delivery is blocked. ", De: "Der Versand des Datenschutzhinweises ist blockiert. ", Vi: "Việc gửi thông báo đang bị chặn. "}) + reason
	}

	return crmcontracts.ContactMoment{
		Rule: crmcontracts.ContactMomentRulePrivacyNoticeDue, RuleVersion: new(ruleVersion),
		ClaimKey: "moment:privacy_notice:" + duty.Id.String(), EvidenceFingerprint: fmt.Sprintf("%s:%s:%t", duty.Id, duty.DueAt.Format(time.RFC3339Nano), duty.Blocked),
		Headline: title, WhyNow: reason, FreshnessAt: &now, Confidence: crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence: []crmcontracts.ContactMomentEvidence{{Type: crmcontracts.ContactMomentEvidenceTypeNoticeCase, Id: &duty.Id, Label: title}},
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:        crmcontracts.ContactMomentActionKindOpenRecord,
			Label:       said.Say(langcopy.Phrase{En: "Review privacy duty", De: "Informationspflicht prüfen", Vi: "Xem nghĩa vụ thông báo"}),
			State:       crmcontracts.ContactMomentActionStateAvailable,
			Destination: &crmcontracts.ContactMomentDestination{Surface: crmcontracts.ContactMomentDestinationSurfacePrivacyNotice, EntityId: &duty.Id},
		},
	}, true
}
