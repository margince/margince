// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The company-sidecar resolvers (commandsidecar.go): the approval binds
// to the company, refuses the same two ways patchResolver's own target
// does, and Subject's summary names the operand — the fact key or the
// profile field — the door-agnostic line GovernedCall.Subject owes this
// operation, distinct per operand even though no door renders it today.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Each sidecar command stages against the COMPANY it routes through,
// with the operand carried into the summary — the property that keeps two
// facts, or two profile fields, on one company from rendering as one
// indistinguishable approval.
func TestSidecarCommandsStageTheCompanyWithTheOperandInTheSummary(t *testing.T) {
	companyID := ids.NewV7()
	cases := []struct {
		name        string
		call        GovernedCall
		wantOperand string
	}{
		{
			"confirm_fact",
			NewConfirmFactCall(unreadableProvider{}, ConfirmFactCommand{ID: companyID, FactKey: "named_customer:acme-inc"}),
			"named_customer:acme-inc",
		},
		{
			"update_fact",
			NewUpdateFactCall(unreadableProvider{}, UpdateFactCommand{ID: companyID, FactKey: "named_customer:acme-inc"}),
			"named_customer:acme-inc",
		},
		{
			"confirm_profile_field",
			NewConfirmProfileFieldCall(unreadableProvider{}, ConfirmProfileFieldCommand{ID: companyID, Field: "icp"}),
			"icp",
		},
		{
			"update_profile_field",
			NewUpdateProfileFieldCall(unreadableProvider{}, UpdateProfileFieldCommand{ID: companyID, Field: "icp"}),
			"icp",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// call.Subject directly, not StageSubject: these fixtures answer every
			// Read with not-found, so a Subject that (wrongly) tried to read the
			// company for a label would fail here — calling Subject alone
			// (skipping Guards, unlike StageSubject) proves it needs no read to
			// name the operand.
			info, err := c.call.Subject(context.Background())
			if err != nil {
				t.Fatalf("naming the subject answered %v, want no error — Subject reads nothing", err)
			}
			if info.TargetType != "company" || info.TargetID != companyID {
				t.Errorf("staged target = (%s,%s), want (company,%s)", info.TargetType, info.TargetID, companyID)
			}
			if !strings.Contains(info.Summary, c.wantOperand) {
				t.Errorf("summary %q does not name the operand %q — Subject owes a line distinct per fact or "+
					"field even though no door renders it today", info.Summary, c.wantOperand)
			}
		})
	}
}

// A company the caller cannot see is refused before anything is
// staged, for all four sidecar commands — the row-scope miss, not merely a
// generic error.
func TestSidecarCommandsRefuseAnUnreadableCompany(t *testing.T) {
	id := ids.NewV7()
	cases := []struct {
		name string
		call GovernedCall
	}{
		{"confirm_fact", NewConfirmFactCall(unreadableProvider{}, ConfirmFactCommand{ID: id, FactKey: "k"})},
		{"update_fact", NewUpdateFactCall(unreadableProvider{}, UpdateFactCommand{ID: id, FactKey: "k"})},
		{"confirm_profile_field", NewConfirmProfileFieldCall(unreadableProvider{}, ConfirmProfileFieldCommand{ID: id, Field: "icp"})},
		{"update_profile_field", NewUpdateProfileFieldCall(unreadableProvider{}, UpdateProfileFieldCommand{ID: id, Field: "icp"})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.call.Guards(context.Background()); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("guarding an unreadable company answered %v, want the row-scope miss", err)
			}
		})
	}
}

// A company held in another system of record is refused too — the
// decidability probe and the version pin both read our own tables, which
// the company has no row in.
func TestSidecarCommandsRefuseAnCompanyHeldElsewhere(t *testing.T) {
	id := ids.NewV7()
	cases := []struct {
		name string
		call GovernedCall
	}{
		{"confirm_fact", NewConfirmFactCall(elsewhereProvider{}, ConfirmFactCommand{ID: id, FactKey: "k"})},
		{"update_fact", NewUpdateFactCall(elsewhereProvider{}, UpdateFactCommand{ID: id, FactKey: "k"})},
		{"confirm_profile_field", NewConfirmProfileFieldCall(elsewhereProvider{}, ConfirmProfileFieldCommand{ID: id, Field: "icp"})},
		{"update_profile_field", NewUpdateProfileFieldCall(elsewhereProvider{}, UpdateProfileFieldCommand{ID: id, Field: "icp"})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.call.Guards(context.Background()); !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
				t.Errorf("guarding a mirrored company answered %v, want the unsupported-by-SoR refusal", err)
			}
		})
	}
}

// A served, readable company is admitted rather than refused — Guards'
// counterpart to the two refusal tests above, proving the happy path through
// the same seam rather than only its failure modes.
func TestSidecarCommandsAdmitAReadableCompany(t *testing.T) {
	id := ids.NewV7()
	provider := stubRecordProvider{rec: stagedRecord(datasource.EntityCompany, id, true)}
	call := NewConfirmFactCall(provider, ConfirmFactCommand{ID: id, FactKey: "k"})
	if err := call.Guards(context.Background()); err != nil {
		t.Fatalf("guarding a readable, authoritative company answered %v, want it admitted", err)
	}
}
