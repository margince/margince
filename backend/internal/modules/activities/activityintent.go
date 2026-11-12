// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func applyPickedTime(field string, wire *crmcontracts.IntentTime, target **time.Time) error {
	if wire == nil {
		return nil
	}
	instant, err := storekit.ResolveIntentTime(field, wire)
	if err != nil {
		return err
	}
	*target = instant
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
