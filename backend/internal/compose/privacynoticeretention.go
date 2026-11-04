// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/privacy"
)

// privacyNoticeRetention hands the privacy page the retention rules the
// retention settings screen shows, so the two never name different windows.
type privacyNoticeRetention struct{ store *privacy.PolicyStore }

// ActingRetention lists the rules that act today. A rule switched off, or held
// back by the retain-only setting, removes nothing and is not disclosed.
func (r privacyNoticeRetention) ActingRetention(ctx context.Context) ([]consent.RetentionRule, error) {
	policies, err := r.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]consent.RetentionRule, 0, len(policies))
	for _, policy := range policies {
		if !policy.Enabled || policy.SuppressedByPosture {
			continue
		}
		out = append(out, consent.RetentionRule{
			Scope: policy.Scope.String(), RetainDays: policy.RetainDays, Action: policy.Action,
		})
	}
	return out, nil
}
