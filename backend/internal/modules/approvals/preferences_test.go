// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import "testing"

func TestMuteableKindsKeepFiniteExpiryAndSharedReview(t *testing.T) {
	for _, kind := range muteableKinds() {
		if ExpiresNever(kind) || selfOnlyKinds[kind] || retiredKinds[kind] || ReleaseSends(kind) {
			t.Errorf("%s cannot safely leave the ordinary queue", kind)
		}
		if _, err := decisionGrantsFor(kind, nil); err != nil {
			t.Errorf("%s lacks a governed decision route: %v", kind, err)
		}
	}
}
