// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
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
	// The two clauses read differently, because selection also asks whether the
	// activity is archived. What they owe each other is the shield: both the
	// floor and the hold, of every activity the original was read into.
	shield := rawCaptureUnshielded(3, 4)
	for _, term := range []string{
		handelsbriefShielded(3, 4),
		retentionscope.HeldThroughAnyLink("a.id"),
	} {
		if !strings.Contains(shield, term) {
			t.Errorf("the delete's shield drops a term:\n%s", shield)
		}
		if !strings.Contains(selector, term) {
			t.Errorf("the selector does not ask a term the delete asks:\n%s", selector)
		}
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
