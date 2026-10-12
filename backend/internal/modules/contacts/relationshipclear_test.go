// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestRelationshipUpdateInputReadsTheNullsItsContractAllows(t *testing.T) {
	t.Parallel()
	in, err := relationshipUpdateInput(crmcontracts.UpdateRelationshipRequest{}, nil, []string{"ended_at", "role", "started_at"})
	if err != nil {
		t.Fatalf("clearing the three nullable fields: %v", err)
	}
	if !in.ClearRole || !in.ClearStartedAt || !in.ClearEndedAt {
		t.Errorf("clears = role %v, started %v, ended %v, want all three", in.ClearRole, in.ClearStartedAt, in.ClearEndedAt)
	}

	untouched, err := relationshipUpdateInput(crmcontracts.UpdateRelationshipRequest{}, nil, nil)
	if err != nil || untouched.ClearRole || untouched.ClearStartedAt || untouched.ClearEndedAt {
		t.Errorf("a patch naming no null cleared something: %+v, %v", untouched, err)
	}
}

func TestRelationshipUpdateInputRefusesANullOnAFieldItCannotClear(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"is_current_primary", "employment_status", "started_precision", "ended_precision"} {
		_, err := relationshipUpdateInput(crmcontracts.UpdateRelationshipRequest{}, nil, []string{field})
		var refusal *storekit.NotClearableError
		if !errors.As(err, &refusal) || refusal.Field != field {
			t.Errorf("%s: null → %v, want a NotClearableError naming the field", field, err)
		}
	}
}

func TestOnlyAnEdgeWhoseRoleIsOptionalCanLoseIt(t *testing.T) {
	t.Parallel()
	if err := refuseRoleClear(employmentKind); err != nil {
		t.Errorf("an employment role is optional: %v", err)
	}
	for _, kind := range []string{BillingContactKind, ProjectStakeholderKind} {
		if err := refuseRoleClear(kind); err == nil {
			t.Errorf("%s: clearing the role was admitted, but the role is what that edge means", kind)
		}
	}
}

func TestServedSegmentsHoldOnlyWords(t *testing.T) {
	t.Parallel()
	words := []string{"fintech", "retail"}
	blank := []string{"fintech", " "}
	empty := []string{""}
	if refuseBlankServedSegments(nil) != nil || refuseBlankServedSegments(&words) != nil {
		t.Error("an absent list or a list of words was refused")
	}
	for _, segments := range [][]string{blank, empty} {
		if refuseBlankServedSegments(&segments) == nil {
			t.Errorf("%q was admitted", segments)
		}
	}
}
