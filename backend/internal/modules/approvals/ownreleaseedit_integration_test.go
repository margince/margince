// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// An agent never edits what it releases: a release is classified on the staged
// payload, so an edit would release something never classified. It is refused
// for a credential's own proposal and for a human's, and the unedited release of
// the credential's own proposal still works.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACredentialDoesNotEditTheProposalItReleasesItself(t *testing.T) {
	e := setupStaging(t)
	e.svc.WithUndoableRelease(func(_ context.Context, kind, _ string, _ json.RawMessage) bool {
		return kind == "company_name_promotion"
	})
	target := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Editable', 'gmail:seed', 'connector:gmail')`, target); err != nil {
		t.Fatalf("seeding the target: %v", err)
	}
	agent := e.lentPassport(t, e.rep)
	staged, err := e.svc.Stage(agent, StageInput{
		Kind:           "company_name_promotion",
		ProposedChange: []byte(`{"proposed_name":"Editable Global"}`),
		DiffHash:       "edit-" + target.String(),
		TargetType:     tableCompany,
		TargetID:       target,
		Summary:        "Rename Editable?",
	})
	if err != nil {
		t.Fatalf("staging: %v", err)
	}

	if _, err := e.svc.DecideEdited(agent, staged, []byte(`{"proposed_name":"Something Else"}`)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a credential released its own proposal with an edited payload → %v, want ErrPermissionDenied", err)
	}
	if _, err := e.svc.Decide(agent, staged, true, nil); err != nil {
		t.Fatalf("the unedited release of the same proposal → %v", err)
	}
}

func TestACredentialDoesNotEditAProposalAHumanStaged(t *testing.T) {
	e := setupStaging(t)
	e.svc.WithUndoableRelease(func(_ context.Context, kind, _ string, _ json.RawMessage) bool {
		return kind == "company_name_promotion"
	})
	target := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Human staged', 'gmail:seed', 'connector:gmail')`, target); err != nil {
		t.Fatalf("seeding the target: %v", err)
	}
	staged, err := e.svc.Stage(e.asRep(e.rep), StageInput{
		Kind:           "company_name_promotion",
		ProposedChange: []byte(`{"proposed_name":"Human Staged Global"}`),
		DiffHash:       "human-edit-" + target.String(),
		TargetType:     tableCompany,
		TargetID:       target,
		Summary:        "Rename Human staged?",
	})
	if err != nil {
		t.Fatalf("staging: %v", err)
	}

	_, err = e.svc.DecideEdited(e.lentPassport(t, e.rep), staged, []byte(`{"proposed_name":"Something Else"}`))
	if !errors.Is(err, apperrors.ErrPermissionDenied) || !strings.Contains(err.Error(), "never edits what it releases") {
		t.Fatalf("a credential edited the proposal its human staged → %v, want the edit refusal", err)
	}
}
