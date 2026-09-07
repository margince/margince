// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The conditions this tree can answer, handed their values directly — because
// the DDL will not store the failing case for one of them.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// germanConditions is what extensions/de declares — a deliberate mirror rather
// than a second answer. It compares this literal against the pack's in both
// directions, so a condition added to one and not the other fails.
//
// Held by: TestTheGermanExceptionFixtureMirrorsThePack (backend/gates/deexceptionmirror_test.go)
//
// Copied rather than imported because extensions/de depends on backend, so
// reading it back here would be a cycle.
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

// The exception Germany ships today refuses before it reads anything.
//
// RequiresSimilarity is set, and similarity is unanswerable with the fields this
// tree has — so existingCustomerAllows short-circuits ahead of the row read.
//
// Without this, the file's other tests exercise conditionsMet — a function the
// SHIPPED German exception never reaches. This is the case that fails the day
// somebody drops RequiresSimilarity from the pack to "make the exception work",
// which is the one change the mirror gate cannot tell from a legitimate one.
func TestTheShippedGermanExceptionRefusesBeforeReadingARow(t *testing.T) {
	german := germanConditions()
	if !german.RequiresSimilarity {
		t.Fatal("the shipped German exception no longer requires similarity. That is the field " +
			"that makes it refuse without reading a row, so this probe would dereference its nil " +
			"transaction — but the change worth noticing is the product one: similarity cannot " +
			"be answered with today's fields, so dropping it grants §7(3) on evidence nobody has")
	}

	// A NIL transaction is the assertion: a query would dereference it, so
	// returning cleanly proves the refusal precedes the read.
	allowed, err := existingCustomerAllows(context.Background(), nil, ids.NewV7().String(), &german)
	if err != nil {
		t.Fatalf("evaluating the shipped German exception: %v", err)
	}
	if allowed {
		t.Error("the shipped §7(3) exception allowed a send. It requires similarity, which this " +
			"tree cannot answer, so the only safe answer is a refusal — and an allow here would " +
			"be one reached without reading the customer's row at all")
	}
}
