// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type inquiryReceipt struct {
	Activity crmcontracts.Activity
	Target   datasource.EntityRef
	Key      string
}

func readInquiryReceipt(ctx context.Context, ex Executors, ev workflow.Event, policy crmcontracts.WebsiteInquiryPolicy) (inquiryReceipt, error) {
	if ex.Provider == nil {
		return inquiryReceipt{}, errors.New("automation: inquiry record reader is not configured")
	}
	admit, err := ownerContext(ctx, ex.Authority, ev.WorkspaceID, ev.OwnerID)
	if err != nil {
		return inquiryReceipt{}, err
	}
	record, err := ex.Provider.Read(admit, ev.Entity)
	if err != nil {
		return inquiryReceipt{}, err
	}
	var activity crmcontracts.Activity
	if err := json.Unmarshal(record.Fields, &activity); err != nil {
		return inquiryReceipt{}, err
	}
	if activity.Kind != crmcontracts.ActivityKindNote || activity.SourceSystem == nil || *activity.SourceSystem != provenance.WebFormSource || activity.SourceId == nil || activity.Raw == nil || activity.Links == nil || activity.ArchivedAt != nil {
		return inquiryReceipt{}, declineFiring("the trigger is not an available accepted website note")
	}
	var payload crmcontracts.PublicEventActivityCaptured
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return inquiryReceipt{}, err
	}
	if payload.WebsiteSubmissionKey == nil || *payload.WebsiteSubmissionKey != *activity.SourceId {
		return inquiryReceipt{}, declineFiring("the event does not name this accepted website note")
	}
	form, _, ok := strings.Cut(*activity.SourceId, ":")
	if !ok || form != policy.FormId.String() {
		return inquiryReceipt{}, declineFiring("the accepted inquiry belongs to another form")
	}
	owner, err := AcceptedInquiryOwner(activity.Raw)
	if err != nil {
		return inquiryReceipt{}, err
	}
	if owner == nil || *owner != ev.AutomationID {
		return inquiryReceipt{}, declineFiring("this handling policy does not own the accepted inquiry")
	}
	var target datasource.EntityRef
	for _, link := range *activity.Links {
		kind := datasource.EntityType(link.EntityType)
		if kind != datasource.EntityLead && kind != datasource.EntityContact {
			continue
		}
		if target.ID != ids.Nil {
			return inquiryReceipt{}, errors.New("automation: inquiry has more than one routing target")
		}
		target = datasource.EntityRef{Type: kind, ID: ids.UUID(link.EntityId)}
	}
	if target.ID == ids.Nil {
		return inquiryReceipt{}, declineFiring("the accepted inquiry has no available lead or contact")
	}
	if _, err := ex.Provider.Read(admit, target); err != nil {
		return inquiryReceipt{}, err
	}
	return inquiryReceipt{Activity: activity, Target: target, Key: *activity.SourceId}, nil
}
