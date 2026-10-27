// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"strings"
	"testing"
)

// Selection and deletion ask one shield of a provider original.
//
// Asked separately they disagreed. Selection tested the candidate activity's
// own floor, deletion tested every linked activity's.
//
// An original read into one expired activity and one shielded sibling was then
// offered every sweep and refused every sweep.
func TestSelectionAndDeletionAskOneShieldOfARawCapture(t *testing.T) {
	selector, ok := retentionSelectors["raw_capture/"]
	if !ok {
		t.Fatal("no raw_capture selector, so this test guards nothing")
	}
	if shield := rawCaptureUnshielded(3, 4); !strings.Contains(selector, shield) {
		t.Errorf("the selector does not ask the shield the delete asks:\n%s", selector)
	}
}

// The shield covers a restricted sibling.
//
// Restriction keeps a record and stops it being processed. Destroying what it
// was read out of is the one thing the restriction was asked to prevent.
func TestTheRawCaptureShieldCoversARestrictedSibling(t *testing.T) {
	shield := rawCaptureUnshielded(3, 4)
	if !strings.Contains(shield, "a.restricted_at IS NOT NULL") {
		t.Errorf("the shield ignores a restricted sibling:\n%s", shield)
	}
}
