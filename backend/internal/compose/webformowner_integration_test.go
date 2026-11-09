// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// formConfigurer is a rep on their own row scope who may set up website forms.
func formConfigurer(e *integration.Env) context.Context {
	perms := handoverRepPerms()
	perms.Objects["integrations"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	return seatedAs(e, e.Rep1, perms)
}

func createFormOwnedBy(t *testing.T, e *integration.Env, owner ids.UUID) error {
	t.Helper()
	_, _, err := webforms.NewStore(e.DB(), slog.Default()).CreateForm(formConfigurer(e), webforms.CreateInput{
		Name: "Website enquiries", Sender: webforms.SenderBrowser,
		AllowedOrigins: []string{"https://visitor.example"}, OwnerID: &owner,
	})
	return err
}

func TestAFormMayHandItsLeadsToAColleagueOutsideTheConfigurersScope(t *testing.T) {
	e := integration.Setup(t)
	if err := createFormOwnedBy(t, e, e.Rep3); err != nil {
		t.Fatalf("naming a colleague outside the configurer's scope as the owner: %v", err)
	}
}

func TestAFormStillRefusesAnAgentSeatAsItsOwner(t *testing.T) {
	e := integration.Setup(t)
	seat := ids.NewV7()
	if _, err := integration.OwnerConn(t).Exec(t.Context(), `
		INSERT INTO app_user (id, email, display_name, status, is_agent)
		VALUES ($1, $2, 'Runner', 'active', true)`,
		seat, "agent-"+seat.String()+"@seat.test"); err != nil {
		t.Fatalf("seeding the agent seat: %v", err)
	}
	err := createFormOwnedBy(t, e, seat)
	if _, refused := errors.AsType[*auth.AssigneeNotAllowedError](err); !refused {
		t.Fatalf("an agent seat as the form's owner got %v, want the owner refusal", err)
	}
}
