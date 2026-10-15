// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The project-stakeholder resolvers (commandaction.go) target `project`, which
// the record seam serves. They refuse the same two ways patchResolver does.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// setStakeholder and removeStakeholder stage against the PROJECT. Only
// removeStakeholder carries a path operand (ContactID) into the summary —
// setStakeholder's contact/role arrive in the body, which the inbox shows
// beside the summary line (proposed_change), the same reasoning patchResolver
// gives for not repeating a patch's values.
func TestStakeholderCommandsStageTheProject(t *testing.T) {
	projectID := ids.NewV7()
	contactID := ids.NewV7()
	// project is served by the seam, so staging it needs a readable provider.
	// An unreadable one would fail at Guards before Subject ever ran.
	provider := stubRecordProvider{rec: stagedRecord(datasource.EntityProject, projectID, true)}

	setInfo, err := StageSubject(context.Background(), NewSetStakeholderCall(provider, nil, SetStakeholderCommand{ID: projectID}))
	if err != nil {
		t.Fatalf("staging a set-stakeholder answered %v, want it staged", err)
	}
	if setInfo.TargetType != "project" || setInfo.TargetID != projectID {
		t.Errorf("staged target = (%s,%s), want (project,%s)", setInfo.TargetType, setInfo.TargetID, projectID)
	}

	removeInfo, err := StageSubject(context.Background(),
		NewRemoveStakeholderCall(provider, nil, RemoveStakeholderCommand{ID: projectID, ContactID: contactID}))
	if err != nil {
		t.Fatalf("staging a remove-stakeholder answered %v, want it staged", err)
	}
	if removeInfo.TargetType != "project" || removeInfo.TargetID != projectID {
		t.Errorf("staged target = (%s,%s), want (project,%s)", removeInfo.TargetType, removeInfo.TargetID, projectID)
	}
	if !strings.Contains(removeInfo.Summary, contactID.String()) {
		t.Errorf("remove-stakeholder summary %q does not name the contact being detached — Subject owes a "+
			"line distinct per contact even though no door renders it today", removeInfo.Summary)
	}
}

// A project the caller cannot see is refused before anything is staged, for
// both stakeholder commands.
func TestStakeholderCommandsRefuseAnUnreadableProject(t *testing.T) {
	id, contactID := ids.NewV7(), ids.NewV7()
	cases := []struct {
		name string
		call GovernedCall
	}{
		{"set", NewSetStakeholderCall(unreadableProvider{}, nil, SetStakeholderCommand{ID: id})},
		{"remove", NewRemoveStakeholderCall(unreadableProvider{}, nil, RemoveStakeholderCommand{ID: id, ContactID: contactID})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.call.Guards(context.Background()); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("guarding an unreadable project answered %v, want the row-scope miss", err)
			}
		})
	}
}

// A project held in another system of record is refused too.
func TestStakeholderCommandsRefuseAProjectHeldElsewhere(t *testing.T) {
	id, contactID := ids.NewV7(), ids.NewV7()
	cases := []struct {
		name string
		call GovernedCall
	}{
		{"set", NewSetStakeholderCall(elsewhereProvider{}, nil, SetStakeholderCommand{ID: id})},
		{"remove", NewRemoveStakeholderCall(elsewhereProvider{}, nil, RemoveStakeholderCommand{ID: id, ContactID: contactID})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.call.Guards(context.Background()); !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
				t.Errorf("guarding a mirrored project answered %v, want the unsupported-by-SoR refusal", err)
			}
		})
	}
}

// A served, readable project is admitted rather than refused.
func TestStakeholderCommandsAdmitAReadableProject(t *testing.T) {
	id, contactID := ids.NewV7(), ids.NewV7()
	provider := stubRecordProvider{rec: stagedRecord(datasource.EntityProject, id, true)}
	if err := NewSetStakeholderCall(provider, nil, SetStakeholderCommand{ID: id}).Guards(context.Background()); err != nil {
		t.Fatalf("guarding a readable, authoritative project (set) answered %v, want it admitted", err)
	}
	if err := NewRemoveStakeholderCall(provider, nil, RemoveStakeholderCommand{ID: id, ContactID: contactID}).Guards(context.Background()); err != nil {
		t.Fatalf("guarding a readable, authoritative project (remove) answered %v, want it admitted", err)
	}
}
