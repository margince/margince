// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetMyBriefDelivery answers what the caller has chosen to receive.
func (h Handlers) GetMyBriefDelivery(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.MyDelivery(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, deliveryToWire(out))
}

// SaveMyBriefDelivery records the caller's choice.
func (h Handlers) SaveMyBriefDelivery(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.BriefDelivery
	if !httperr.Decode(w, r, &body) {
		return
	}
	out, err := h.svc.SaveMyDelivery(r.Context(), deliveryFromWire(body))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, deliveryToWire(out))
}

// deliveryToWire puts the settings on the contract's shape.
//
// An unchosen setting is ABSENT rather than rendered as its default: the
// distinction between "never chose" and "chose none" is the whole point of the
// column being nullable, and a wire shape that filled one in would throw it
// away at the last step.
func deliveryToWire(d Delivery) crmcontracts.BriefDelivery {
	out := crmcontracts.BriefDelivery{
		QuietDayNotice:      d.QuietDay,
		RoutineDeliveryTime: d.RoutineTime,
		DeliveryHourLocal:   d.HourLocal,
		WeeklyDeliveryDay:   d.WeeklyDay,
		CombineRoutineEmail: d.CombineRoutine,
	}
	if d.Scope != nil {
		scope := crmcontracts.BriefDeliveryRoutineMailScope(*d.Scope)
		out.RoutineMailScope = &scope
	}
	if d.TeamID != nil {
		team := openapi_types.UUID(*d.TeamID)
		out.RoutineMailTeamId = &team
	}
	if d.MorningBrief != nil {
		choice := crmcontracts.BriefDeliveryMorningBriefDelivery(*d.MorningBrief)
		out.MorningBriefDelivery = &choice
	}
	if d.Weekly != nil {
		choice := crmcontracts.BriefDeliveryWeeklyDelivery(*d.Weekly)
		out.WeeklyDelivery = &choice
	}
	return out
}

// deliveryFromWire reads a patch off the wire.
//
// An omitted field stays nil and the service leaves it alone. The generated
// type is all pointers, so "not sent" and "sent as empty" are already distinct
// before this is reached.
func deliveryFromWire(body crmcontracts.BriefDelivery) Delivery {
	out := Delivery{RoutineTime: body.RoutineDeliveryTime, QuietDay: body.QuietDayNotice, HourLocal: body.DeliveryHourLocal, WeeklyDay: body.WeeklyDeliveryDay, CombineRoutine: body.CombineRoutineEmail}
	if body.RoutineMailScope != nil {
		scope := string(*body.RoutineMailScope)
		out.Scope = &scope
	}
	if body.RoutineMailTeamId != nil {
		team := ids.UUID(*body.RoutineMailTeamId)
		out.TeamID = &team
	}
	if body.MorningBriefDelivery != nil {
		choice := string(*body.MorningBriefDelivery)
		out.MorningBrief = &choice
	}
	if body.WeeklyDelivery != nil {
		choice := string(*body.WeeklyDelivery)
		out.Weekly = &choice
	}
	return out
}
