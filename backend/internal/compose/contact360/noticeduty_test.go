// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"context"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUnresolvedPrivacyDutyCannotBecomeNothingNeeded(t *testing.T) {
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	id := openapi_types.UUID(ids.NewV7())
	page := &crmcontracts.Contact360{PrivacyNoticeDuties: &[]crmcontracts.ContactNoticeDuty{{Id: id, Rule: crmcontracts.ContactNoticeDutyRuleArt14, DueAt: now.Add(time.Hour)}}}
	said := momentCopyFor("en")
	moment := said.deriveMomentPast(context.Background(), now, page, func(crmcontracts.ContactMoment) bool { return true }, asAssembled)
	if moment.Rule != crmcontracts.ContactMomentRulePrivacyNoticeDue || moment.RecommendedAction.Destination == nil || moment.RecommendedAction.Destination.EntityId == nil || *moment.RecommendedAction.Destination.EntityId != id {
		t.Fatalf("privacy duty was lost: %+v", moment)
	}
	if moment.RecommendedAction.Destination.Surface != crmcontracts.ContactMomentDestinationSurfacePrivacyNotice {
		t.Fatalf("privacy action opens %s", moment.RecommendedAction.Destination.Surface)
	}
	(*page.PrivacyNoticeDuties)[0].Blocked = true
	blocked := said.deriveMoment(context.Background(), now, page)
	if !strings.Contains(blocked.WhyNow, "blocked") || blocked.Evidence[0].Type != crmcontracts.ContactMomentEvidenceTypeNoticeCase || blocked.Evidence[0].Id == nil {
		t.Fatalf("blocked duty lost its evidence: %+v", blocked)
	}
	page.PrivacyNoticeDuties = &[]crmcontracts.ContactNoticeDuty{}
	if quiet := said.deriveMoment(context.Background(), now, page); quiet.Rule == crmcontracts.ContactMomentRulePrivacyNoticeDue {
		t.Fatal("a resolved duty still asks for a privacy notice")
	}
}
