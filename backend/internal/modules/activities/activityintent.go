// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"time"
)

func applyPickedTime(field string, wire *crmcontracts.IntentTime, target **time.Time) error {
	if wire == nil {
		return nil
	}
	zone, err := values.ParseTimezone(wire.Timezone)
	if err != nil {
		return &values.ParseError{Field: field + ".timezone", Code: "invalid", Message: "choose an IANA timezone"}
	}
	intent, err := values.ParseIntentTime(wire.WallClock, zone)
	if err != nil {
		return &values.ParseError{Field: field + ".wall_clock", Code: "invalid", Message: "choose a valid local date and time"}
	}
	instant, err := intent.Resolve()
	if err != nil {
		return err
	}
	*target = &instant
	return nil
}

func (in *LogActivityInput) admitPickedTimes() error {
	if err := applyPickedTime("due_intent", in.DueIntent, &in.DueAt); err != nil {
		return err
	}
	return applyPickedTime("remind_intent", in.RemindIntent, &in.RemindAt)
}

func (in *UpdateActivityInput) admitPickedTimes() error {
	if err := applyPickedTime("due_intent", in.DueIntent, &in.DueAt); err != nil {
		return err
	}
	return applyPickedTime("remind_intent", in.RemindIntent, &in.RemindAt)
}

func pickedWall(intent *crmcontracts.IntentTime) *string {
	if intent == nil {
		return nil
	}
	return &intent.WallClock
}

func pickedZone(intent *crmcontracts.IntentTime) *string {
	if intent == nil {
		return nil
	}
	return &intent.Timezone
}
