// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// Own-release is classified on the payload the credential staged, so an edit
// would release something never classified: it is refused for an agent, and the
// unedited release of the same proposal still works.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACredentialDoesNotEditTheProposalItReleasesItself(t *testing.T) {
	e := setupStaging(t)
	e.svc.WithUndoableRelease(func(kind, _ string, _ json.RawMessage) bool { return kind == "company_name_promotion" })
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
