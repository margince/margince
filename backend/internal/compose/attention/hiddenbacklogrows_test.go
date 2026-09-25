// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The projection behind "show me what that figure counted".
//
// The difference itself is the module's and is asserted against a real database
// beside the query. What is asserted here is the part compose owns: that an
// unbound lane answers rather than refuses, that a record the thread does not
// name is absent rather than a zero uuid, and that the rule the caller asked
// for is echoed back.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rowsSeam answers a fixed set of rows and records the rule it was asked for.
type rowsSeam struct {
	hidingWork
	rows  []WaitingCustomer
	asked string
	err   error
}

func (r *rowsSeam) HiddenRows(_ context.Context, _ time.Time, rule string) ([]WaitingCustomer, error) {
	r.asked = rule
	if r.err != nil {
		return nil, r.err
	}
	return r.rows, nil
}

// An installation that does not read the mail stream has no queue to hide work
// from, so an empty list is the true answer rather than a degraded one — the
// same answer the counts give, for the same reason.
func TestAnUnboundLaneListsNothingRatherThanRefusing(t *testing.T) {
	t.Parallel()

	got, err := unboundService().HiddenBacklogRows(aLead(), "set_aside")

	if err != nil {
		t.Fatalf("an unbound lane should answer, not refuse: %v", err)
	}
	if len(got.Rows) != 0 {
		t.Errorf("listed %d rows from a lane that never ran", len(got.Rows))
	}
	// Empty rather than null: a client reading `rows` must not have to tell an
	// absent list from an empty one to know nothing is hidden.
	if got.Rows == nil {
		t.Error("answered a null row list where an empty one is the fact")
	}
	if string(got.Rule) != "set_aside" {
		t.Errorf("echoed rule %q, want the one asked for", got.Rule)
	}
}

// The rule travels to the seam unchanged. The counts and the rows have to agree
// about which rule is which, and the word is how they agree.
func TestTheRuleAskedForReachesTheSeamAndComesBack(t *testing.T) {
	t.Parallel()

	seam := &rowsSeam{}
	svc := unboundService()
	svc.waiting = seam

	got, err := svc.HiddenBacklogRows(aLead(), "past_horizon")
	if err != nil {
		t.Fatal(err)
	}

	if seam.asked != "past_horizon" {
		t.Errorf("asked the seam for %q, want the caller's rule", seam.asked)
	}
	if string(got.Rule) != "past_horizon" {
		t.Errorf("echoed %q, want the caller's rule", got.Rule)
	}
}

// A record the thread does not name is ABSENT, not a zero uuid. A zero on the
// wire is an id a client could try to open, and it would open nothing.
func TestARecordTheThreadDoesNotNameIsAbsentRatherThanZero(t *testing.T) {
	t.Parallel()

	contact := ids.NewV7()
	seam := &rowsSeam{rows: []WaitingCustomer{{
		ActivityID: ids.NewV7(),
		Subject:    "Question about pricing",
		Since:      time.Unix(0, 0).UTC(),
		ContactID:  contact,
		// CompanyID and DealID left zero: this thread names neither.
	}}}
	svc := unboundService()
	svc.waiting = seam

	got, err := svc.HiddenBacklogRows(aLead(), "unlinked")
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Rows) != 1 {
		t.Fatalf("listed %d rows, want the one the seam answered", len(got.Rows))
	}
	row := got.Rows[0]
	if row.ContactId == nil || ids.UUID(*row.ContactId) != contact {
		t.Errorf("contact_id = %v, want the one the thread names", row.ContactId)
	}
	if row.CompanyId != nil {
		t.Errorf("company_id = %v, want absent — the thread names no company", *row.CompanyId)
	}
	if row.DealId != nil {
		t.Errorf("deal_id = %v, want absent — the thread names no deal", *row.DealId)
	}
}

// A seam that fails fails the read. A list that swallowed the error would tell
// a reader nothing is behind a figure they can see is not zero.
func TestAFailedSeamReadIsNotAnEmptyList(t *testing.T) {
	t.Parallel()

	wanted := errors.New("the lane is unreachable")
	svc := unboundService()
	svc.waiting = &rowsSeam{err: wanted}

	_, err := svc.HiddenBacklogRows(aLead(), "not_sales")

	if !errors.Is(err, wanted) {
		t.Fatalf("err = %v, want the seam's own", err)
	}
}
