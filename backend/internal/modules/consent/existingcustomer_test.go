// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The conditions this tree can answer, handed their values directly — because
// the DDL will not store the failing case for one of them.

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// germanConditions is what extensions/de declares.
func germanConditions() messaging.MarketingException {
	return messaging.MarketingException{
		Kind:                         messaging.ExistingCustomer,
		RequiresSaleEvidence:         true,
		RequiresCollectionTimeOptOut: true,
		RequiresSimilarity:           true,
		RequiresNoObjection:          true,
	}
}

// TestTheNoticeConditionIsAskedOfTheRow reaches what the schema will not store.
//
// consent_existing_customer_notice refuses a row with optout_notice_given
// false, so no integration case can produce one — but the condition is the
// pack's to require, and a check nothing exercises is a check nobody can trust.
func TestTheNoticeConditionIsAskedOfTheRow(t *testing.T) {
	all := germanConditions()
	if conditionsMet(all, "INV-1", false) {
		t.Error("a sale collected without the opt-out notice satisfied the exception: §7(3) " +
			"requires the notice at collection, and the DDL not storing that shape today is " +
			"not the same as this code asking for it")
	}
	// The control: the same row WITH the notice allows, so the case above fails
	// for the notice and not for something else.
	if !conditionsMet(all, "INV-1", true) {
		t.Error("the same evidence with the notice on file did not allow")
	}
	// A pack that does not require it gets the row through either way.
	relaxed := all
	relaxed.RequiresCollectionTimeOptOut = false
	if !conditionsMet(relaxed, "INV-1", false) {
		t.Error("a pack that does not require the notice was refused for want of it: the " +
			"condition is the pack's to declare, not this function's to assume")
	}
}

// TestSaleEvidenceMustBeRecorded — NOT NULL is not the same as recorded.
func TestSaleEvidenceMustBeRecorded(t *testing.T) {
	all := germanConditions()
	for _, empty := range []string{"", "   "} {
		if conditionsMet(all, empty, true) {
			t.Errorf("a flag row whose sale reference is %q satisfied the exception: the column "+
				"is NOT NULL and an empty string passes it, so a form that skipped the field "+
				"writes a row that evidences nothing", empty)
		}
	}
	if !conditionsMet(all, "INV-1", true) {
		t.Error("a recorded sale reference did not allow")
	}
}
