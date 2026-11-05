// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/providerwait"
)

func TestVoiceDeferralWireKeepsLegacyDetailAndExplainsTheWait(t *testing.T) {
	code := "budget_deferred"
	for _, tc := range []struct{ detail, reason string }{{providerwait.Detail, "provider"}, {"Budget limit", "budget"}} {
		wire := wireVoiceBuild(VoiceBuild{Status: "deferred", StatusCode: &code, StatusDetail: &tc.detail})
		if wire.DeferralReason == nil || *wire.DeferralReason != tc.reason || wire.StatusDetail == nil || *wire.StatusDetail != tc.detail {
			t.Fatalf("lost retry cause or compatibility detail: %+v", wire)
		}
	}
}
