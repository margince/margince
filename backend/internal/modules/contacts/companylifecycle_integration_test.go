// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asNonHuman is the rep's own grants worn by a caller that is not a signed-in
// user, so only the principal type tells the two apart.
func (e *dedupeEnv) asNonHuman(kind principal.PrincipalType) context.Context {
	human, ok := principal.Actor(e.as())
	if !ok {
		panic("the rep context carries no actor")
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: kind, ID: string(kind) + ":" + e.rep.String(), Permissions: human.Permissions,
	})
}

func (e *dedupeEnv) companyAtStage(ctx context.Context, t *testing.T, name, lifecycle string) ids.CompanyID {
	t.Helper()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{Lifecycle: &lifecycle}); err != nil {
		t.Fatalf("setting %s to %s: %v", name, lifecycle, err)
	}
	return id
}

func (e *dedupeEnv) lifecycleOf(ctx context.Context, t *testing.T, id ids.CompanyID) string {
	t.Helper()
	var stage string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT lifecycle FROM company WHERE id = $1`, id).Scan(&stage)
	}); err != nil {
		t.Fatal(err)
	}
	return stage
}

func TestAUserWalksAnAccountFromOpportunityThroughLiveToCustomer(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	id := e.companyAtStage(ctx, t, "Walk GmbH", "opportunity")
	for _, stage := range []string{"onboarding", "live", "customer"} {
		audits := e.countInWorkspace(ctx, t,
			`SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = $1`, id)
		events := countCompanyUpdatedEvents(ctx, t, e, id)
		next := stage
		updated, err := e.store.UpdateCompany(e.as(), id, UpdateCompanyInput{Lifecycle: &next})
		if err != nil {
			t.Fatalf("moving to %s: %v", stage, err)
		}
		if updated.Lifecycle == nil || string(*updated.Lifecycle) != stage {
			t.Fatalf("lifecycle = %v, want %s", updated.Lifecycle, stage)
		}
		if got := e.countInWorkspace(ctx, t,
			`SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = $1`, id); got != audits+1 {
			t.Errorf("moving to %s wrote %d audit rows, want 1", stage, got-audits)
		}
		if got := countCompanyUpdatedEvents(ctx, t, e, id); got != events+1 {
			t.Errorf("moving to %s emitted %d company.updated events, want 1", stage, got-events)
		}
	}
}

func TestNoAgentAutomationOrConnectorSetsAnAccountLive(t *testing.T) {
	e := setupDedupe(t)
	id := e.companyAtStage(e.as(), t, "Not Yet Live GmbH", "onboarding")
	for _, kind := range []principal.PrincipalType{principal.PrincipalAgent, principal.PrincipalSystem, principal.PrincipalConnector} {
		t.Run(string(kind), func(t *testing.T) {
			live := "live"
			_, err := e.store.UpdateCompany(e.asNonHuman(kind), id, UpdateCompanyInput{Lifecycle: &live})
			requireLiveRefusal(t, err)
			if got := e.lifecycleOf(e.as(), t, id); got != "onboarding" {
				t.Errorf("the refused write left the account at %s, want onboarding", got)
			}
		})
	}
	// Onboarding is an ordinary stage: an agent may set it.
	onboarding := "onboarding"
	if _, err := e.store.UpdateCompany(e.asNonHuman(principal.PrincipalAgent), e.companyAtStage(e.as(), t, "Signed GmbH", "opportunity"),
		UpdateCompanyInput{Lifecycle: &onboarding}); err != nil {
		t.Fatalf("an agent setting onboarding was refused: %v", err)
	}
}

func TestAnApprovedProposalNeverMovesAnAccountLive(t *testing.T) {
	e := setupDedupe(t)
	id := e.companyAtStage(e.as(), t, "Proposed GmbH", "onboarding")
	ctx := principal.SystemActing(e.as(), "approval-executor")
	err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.SetCompanyLifecycleTx(ctx, tx, id, "onboarding", "live")
		return err
	})
	requireLiveRefusal(t, err)
	if got := e.lifecycleOf(e.as(), t, id); got != "onboarding" {
		t.Errorf("the refused move left the account at %s, want onboarding", got)
	}
}

func TestSettingLiveKeepsTheRowAndObjectGates(t *testing.T) {
	e := setupDedupe(t)
	id := e.companyAtStage(e.as(), t, "Gated GmbH", "onboarding")
	live := "live"
	if _, err := e.store.UpdateCompany(e.asCompanyReader(), id, UpdateCompanyInput{Lifecycle: &live}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a read-only seat setting live: err = %v, want permission denied", err)
	}
	if _, err := e.store.UpdateCompany(asAdmin(e), id, UpdateCompanyInput{OwnerID: new(ids.From[ids.UserKind](e.otherRep))}); err != nil {
		t.Fatal(err)
	}
	ownOnly, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("the rep context carries no actor")
	}
	ownOnly.Permissions.RowScope = principal.RowScopeOwn
	ctx := principal.WithActor(e.as(), ownOnly)
	// Everyone reads a workspace company, so a colleague's is refused, not hidden.
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{Lifecycle: &live}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("setting live on a colleague's company: err = %v, want permission denied", err)
	}
	ownerOnly := "owner"
	if _, err := e.store.UpdateCompany(asAdmin(e), id, UpdateCompanyInput{Visibility: &ownerOnly}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateCompany(ctx, id, UpdateCompanyInput{Lifecycle: &live}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("setting live on a company the caller cannot see: err = %v, want not found", err)
	}
	if got := e.lifecycleOf(asAdmin(e), t, id); got != "onboarding" {
		t.Errorf("a refused write left the account at %s, want onboarding", got)
	}
}

func TestTheLifecycleFilterReachesOnboardingAndLive(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	want := map[string]ids.CompanyID{
		"onboarding": e.companyAtStage(ctx, t, "Setting Up GmbH", "onboarding"),
		"live":       e.companyAtStage(ctx, t, "In Use GmbH", "live"),
	}
	e.companyAtStage(ctx, t, "Plain Customer GmbH", "customer")
	for stage, id := range want {
		filter := stage
		found, _, err := e.store.ListCompanies(ctx, ListCompaniesInput{Lifecycle: &filter})
		if err != nil {
			t.Fatalf("listing %s: %v", stage, err)
		}
		if len(found) != 1 || found[0].Id.String() != id.String() {
			t.Errorf("lifecycle=%s listed %d companies, want only %s", stage, len(found), id)
		}
	}
}

func requireLiveRefusal(t *testing.T, err error) {
	t.Helper()
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Code != "lifecycle_live_needs_a_human" {
		t.Fatalf("want a lifecycle_live_needs_a_human refusal, got %v", err)
	}
}
