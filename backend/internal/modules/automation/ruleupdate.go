// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// settleRuleOwner hands a rule on where the update names an owner, and claims a
// draft starter for whoever enables it. It answers the owner it claimed, if any.
func settleRuleOwner(ctx context.Context, tx pgx.Tx, before Automation, in UpdateAutomationInput) (ids.UUID, error) {
	if in.OwnerID != nil {
		var current *ids.UUID
		if before.OwnerID != nil {
			current = &before.OwnerID.UUID
		}
		if err := auth.EnsureRunAsOwnerHandOn(ctx, tx, current, in.OwnerID); err != nil {
			return ids.Nil, err
		}
	}
	enabled := before.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if !enabled || (before.Enabled && (in.Params == nil || sameParams(before.Params, in.Params))) {
		return ids.Nil, nil
	}
	if in.Params != nil {
		params, err := json.Marshal(in.Params)
		if err != nil {
			return ids.Nil, err
		}
		before.Params = params
	}
	return claimStarterAuthorizer(ctx, tx, before)
}

// ruleUpdateImages is an update's before and after, with the owner and setter only
// where they are at stake.
func ruleUpdateImages(before, after Automation, in UpdateAutomationInput) (beforeImage, afterImage map[string]any) {
	beforeImage = map[string]any{keyName: before.Name, auditEnabled: before.Enabled, keyParams: json.RawMessage(before.Params)}
	afterImage = map[string]any{keyName: after.Name, auditEnabled: after.Enabled, keyParams: json.RawMessage(after.Params)}
	if in.OwnerID != nil {
		beforeImage[keyOwnerID], afterImage[keyOwnerID] = before.OwnerID, after.OwnerID
	}
	if before.SetterPassportID != nil || after.SetterPassportID != nil {
		beforeImage[keySetterPassport], afterImage[keySetterPassport] = before.SetterPassportID, after.SetterPassportID
	}
	return beforeImage, afterImage
}
