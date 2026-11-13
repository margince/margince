// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// The two recipients a configured task can go to.
const (
	recipientRecordOwner = "record_owner"
	recipientAuthorizer  = "authorizer"
)

func validateTaskRecipientParam(params map[string]any) error {
	value, present := params[paramRecipient]
	if !present {
		return nil
	}
	if value != recipientRecordOwner && value != recipientAuthorizer {
		return &ParamError{Field: "params.recipient", Reason: "choose the record owner or the person authorizing the automation"}
	}
	return nil
}

func configuredTaskEffect(ctx context.Context, ex Executors, ev workflow.Event, subject string, dueAt time.Time) (workflow.Effect, error) {
	var params struct {
		Recipient string `json:"recipient"`
	}
	if len(ev.Params) > 0 {
		if err := json.Unmarshal(ev.Params, &params); err != nil {
			return workflow.Effect{}, err
		}
	}
	if params.Recipient == recipientAuthorizer {
		if ev.OwnerID.IsZero() {
			return workflow.Effect{}, workflow.Declined("the person authorizing the automation is unavailable")
		}
		return taskCreateEffectFor(ctx, ev, subject, dueAt, &ev.OwnerID, nil)
	}
	return ownedTaskEffectNoKey(ctx, ex, ev, subject, dueAt)
}

func taskRecipientSchema() map[string]any {
	return map[string]any{
		schemaKeyType:        schemaTypeString,
		schemaKeyEnum:        []string{recipientRecordOwner, recipientAuthorizer},
		schemaKeyDescription: "Who receives the task.",
	}
}
