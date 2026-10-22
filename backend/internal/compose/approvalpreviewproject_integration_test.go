// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestProjectPhaseApprovalDescribesTheActualWriter(t *testing.T) {
	for _, reopen := range []bool{false, true} {
		for _, rest := range []bool{false, true} {
			t.Run(fmt.Sprintf("reopen=%t/rest=%t", reopen, rest), func(t *testing.T) {
				e := integration.Setup(t)
				ctx := e.Admin()
				store := e.Projects
				company := e.SeedCompany(t, "Harbor", &e.AdminUser)
				project, err := store.CreateProject(ctx, projects.CreateProjectInput{Name: "Migration", CompanyID: ids.From[ids.CompanyKind](company), Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				projectID := ids.From[ids.ProjectKind](ids.UUID(project.Id))
				from, to, reason, wantedReason := "initiative", "closed", "  Delivered  ", "Delivered"
				if reopen {
					if _, err := store.AdvanceProjectPhase(ctx, projectID, projects.AdvanceProjectPhaseInput{ToPhase: "closed", Reason: new("Delivered")}); err != nil {
						t.Fatal(err)
					}
					from, to, reason, wantedReason = "closed", "delivering", "New scope", ""
				}
				payload := fmt.Sprintf(`{"project_id":%q,"to_phase":%q,"reason":%q}`, project.Id, to, reason)
				if rest {
					payload = fmt.Sprintf(`{"operation":"advanceProjectPhase","path":"/v1/projects/%s/advance","body":{"to_phase":%q,"reason":%q}}`, project.Id, to, reason)
				}
				raw, hash, err := diffhash.Canonical([]byte(payload))
				if err != nil {
					t.Fatal(err)
				}
				svc := NewApprovalsService(e.DB())
				id, err := svc.Stage(ctx, approvals.StageInput{Kind: "advance_project_phase", TargetType: "project", TargetID: ids.UUID(project.Id), ProposedChange: raw, DiffHash: hash})
				if err != nil {
					t.Fatal(err)
				}
				proposal, err := svc.GetWire(ctx, id)
				if err != nil || proposal.Preview == nil || proposal.Preview.State != "ready" {
					t.Fatalf("preview=%+v, %v", proposal.Preview, err)
				}
				if len(proposal.Preview.Changes) != 2 {
					t.Fatalf("changes=%+v", proposal.Preview.Changes)
				}
				for _, change := range proposal.Preview.Changes {
					switch change.Field {
					case "phase":
						if change.Before.Text != from || change.After.Text != to {
							t.Fatalf("phase=%+v", change)
						}
					case "closed_reason":
						if change.After.Text != wantedReason {
							t.Fatalf("reason=%+v", change)
						}
					default:
						t.Fatalf("unexpected change=%+v", change)
					}
				}
				if _, err := svc.Decide(ctx, id, true, nil); err != nil {
					t.Fatal(err)
				}
				changed, err := store.AdvanceProjectPhase(ctx, projectID, projects.AdvanceProjectPhaseInput{ToPhase: to, Reason: &reason})
				if err != nil {
					t.Fatal(err)
				}
				if changed.Phase == nil || string(*changed.Phase) != to {
					t.Fatalf("phase=%+v", changed.Phase)
				}
				if wantedReason == "" {
					if changed.ClosedReason != nil {
						t.Fatalf("closed reason retained: %q", *changed.ClosedReason)
					}
				} else if changed.ClosedReason == nil || *changed.ClosedReason != wantedReason {
					t.Fatalf("reason=%+v", changed.ClosedReason)
				}
			})
		}
	}
}

func TestProjectPhaseApprovalRefusesIncompleteAndChangedEffects(t *testing.T) {
	for _, tc := range []struct{ name, phase, reason, state string }{
		{"unknown phase", "unsupported", "Reason", "incomplete"},
		{"missing close reason", "closed", " ", "incomplete"},
		{"changed project", "closed", "Delivered", "stale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			company := e.SeedCompany(t, "Harbor", &e.AdminUser)
			project, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{Name: "Migration", CompanyID: ids.From[ids.CompanyKind](company), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			projectID := ids.From[ids.ProjectKind](ids.UUID(project.Id))
			raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"project_id":%q,"to_phase":%q,"reason":%q}`, project.Id, tc.phase, tc.reason)))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "advance_project_phase", TargetType: "project", TargetID: ids.UUID(project.Id), ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			expectedPhase := "initiative"
			if tc.state == "stale" {
				expectedPhase = "delivering"
				if _, err := e.Projects.AdvanceProjectPhase(ctx, projectID, projects.AdvanceProjectPhaseInput{ToPhase: expectedPhase}); err != nil {
					t.Fatal(err)
				}
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil || proposal.Preview == nil || proposal.Preview.State != tc.state || len(proposal.Preview.Changes) != 0 {
				t.Fatalf("preview=%+v, %v", proposal.Preview, err)
			}
			_, err = svc.Decide(ctx, id, true, nil)
			if tc.state == "stale" {
				if !errors.Is(err, apperrors.ErrVersionSkew) {
					t.Fatalf("stale decision: %v", err)
				}
			} else if _, ok := errors.AsType[*approvals.PreviewContextError](err); !ok {
				t.Fatalf("incomplete decision: %v", err)
			}
			current, err := e.Projects.GetProject(ctx, projectID, storekit.LiveOnly)
			if err != nil || current.Phase == nil || string(*current.Phase) != expectedPhase {
				t.Fatalf("refusal mutated project: %+v, %v", current, err)
			}
			if _, err := svc.Decide(ctx, id, false, nil); err != nil {
				t.Fatalf("rejection refused: %v", err)
			}
		})
	}
}
