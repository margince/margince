// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOneWorkstreamOrUnknownScopeDoesNotProposeEndingTheAccount(t *testing.T) {
	for _, scope := range []string{"workstream", "unknown", ""} {
		t.Run(scope, func(t *testing.T) {
			e := integration.Setup(t)
			company := seedAccountAtStage(t, e, "customer")
			signal := seedScopedContractEnded(t, e, company, scope, "We are ending the Atlas pilot.")
			if staged := proposePass(t, e); staged != 0 {
				t.Fatal("scoped or uncertain evidence proposed ending the whole account")
			}
			if signalStatus(t, e, signal) != "open" || accountStage(t, e, company) != "customer" {
				t.Fatal("unresolved scope lost its reviewable signal or changed the account")
			}
		})
	}
}

func TestChangedSourceCannotReleaseACompanyTermination(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	signal := seedOpenContractEnded(t, e, company)
	if proposePass(t, e) != 1 {
		t.Fatal("fixture did not stage the account-wide proposal")
	}
	reader := e.As(e.Rep1, nil, integration.AdminWithSignals)
	svc := approvalsServiceWithEffects(e.Pool)
	proposalID := ids.From[ids.ApprovalKind](stagedOffer(t, e, company))
	before, err := svc.GetWire(reader, proposalID)
	if err != nil || before.Preview == nil || before.Preview.State != previewReadyState {
		t.Fatalf("current source preview: %+v, %v", before.Preview, err)
	}
	proof := before.Preview.LifecycleEvidence
	if proof == nil || proof.Scope != "company" || !proof.OccurredAt.Equal(time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("preview lost source date or account scope: %+v", proof)
	}
	source, err := ids.Parse(e.WsScalar(t, `SELECT evidence->0->>'source_id' FROM signal WHERE id=$1`, signal))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](source), activities.UpdateActivityInput{
		Body: new("Only the Atlas pilot is ending. Support continues."),
	}); err != nil {
		t.Fatal(err)
	}
	after, err := svc.GetWire(reader, proposalID)
	if err != nil || after.Preview == nil || after.Preview.State != previewStaleState {
		t.Fatalf("changed source preview: %+v, %v", after.Preview, err)
	}
	_, err = svc.Decide(reader, proposalID, true, nil)
	if err != nil && !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatal(err)
	}
	if accountStage(t, e, company) != "customer" || signalStatus(t, e, signal) != "open" {
		t.Fatal("stale source changed the lifecycle or acknowledged its signal")
	}
}

func TestLifecycleUsesSourceOrderInsteadOfExtractionOrder(t *testing.T) {
	for _, scope := range []string{"company", "workstream", "unknown"} {
		t.Run(scope, func(t *testing.T) {
			e := integration.Setup(t)
			company := seedAccountAtStage(t, e, "customer")
			older := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
			newer := older.AddDate(0, 0, 10)
			seedScopedContractEndedAt(t, e, company, "company", "We ended all business last month.", older, newer.AddDate(0, 0, 10))
			latest := seedScopedContractEndedAt(t, e, company, scope, "Our current termination decision.", newer, newer)
			staged := proposePass(t, e)
			if scope != "company" {
				if staged != 0 {
					t.Fatal("late extraction of older mail overrode newer scoped or unresolved evidence")
				}
				return
			}
			if staged != 1 {
				t.Fatalf("whole-company source should remain actionable, got %d offers", staged)
			}
			selected := e.WsScalar(t, `SELECT proposed_change->>'signal_id' FROM approval WHERE id = $1`, stagedOffer(t, e, company))
			if selected != latest.String() {
				t.Fatalf("selected signal %s, want newest source %s", selected, latest)
			}
		})
	}
}

func TestSimultaneousConflictingScopesNeedReview(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	at := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	seedScopedContractEndedAt(t, e, company, "company", "We are ending all work.", at, at)
	seedScopedContractEndedAt(t, e, company, "workstream", "Only the pilot is ending.", at, at.Add(time.Hour))
	if staged := proposePass(t, e); staged != 0 {
		t.Fatal("equally dated conflicting scope must not choose an account-wide change by signal ID")
	}
}

func TestAnActiveAgreementPreventsEndingTheAccount(t *testing.T) {
	for _, stagedFirst := range []bool{false, true} {
		name := "existing agreement"
		if stagedFirst {
			name = "agreement after proposal"
		}
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			company := seedAccountAtStage(t, e, "customer")
			signal := seedOpenContractEnded(t, e, company)
			if stagedFirst && proposePass(t, e) != 1 {
				t.Fatal("fixture did not stage the initial termination")
			}
			agreement, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{
				CompanyID: ids.From[ids.CompanyKind](company), Title: "Ongoing support", ValueBasis: contracts.BasisTotal, Source: "manual",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Contracts.ChangeStatus(e.Admin(), ids.From[ids.ContractKind](ids.UUID(agreement.Id)), contracts.StatusActive, nil); err != nil {
				t.Fatal(err)
			}
			if !stagedFirst {
				if proposePass(t, e) != 0 {
					t.Fatal("active support agreement allowed account-wide termination")
				}
				return
			}
			reader := e.As(e.Rep1, nil, integration.AdminWithSignals)
			svc := approvalsServiceWithEffects(e.Pool)
			proposal := ids.From[ids.ApprovalKind](stagedOffer(t, e, company))
			view, err := svc.GetWire(reader, proposal)
			if err != nil || view.Preview == nil || view.Preview.State != previewStaleState {
				t.Fatalf("new agreement did not invalidate preview: %+v, %v", view.Preview, err)
			}
			conflicts := view.Preview.Conflicts
			if conflicts == nil || len(*conflicts) != 1 || (*conflicts)[0].Kind != "active_contract" || !(*conflicts)[0].BlocksApproval {
				t.Fatalf("preview did not explain the continuing agreement: %+v", conflicts)
			}
			_, err = svc.Decide(reader, proposal, true, nil)
			if err != nil && !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatal(err)
			}
			if accountStage(t, e, company) != "customer" || signalStatus(t, e, signal) != "open" {
				t.Fatal("new agreement was ignored during approval")
			}
		})
	}
}

func TestOnlyProjectDeliveryBlocksEndingTheAccount(t *testing.T) {
	for _, phase := range []string{projects.PhaseInitiative, projects.PhasePursuing, projects.PhaseDelivering, projects.PhaseClosed} {
		t.Run(phase, func(t *testing.T) {
			e := integration.Setup(t)
			company := seedAccountAtStage(t, e, "customer")
			seedOpenContractEnded(t, e, company)
			project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
				CompanyID: ids.From[ids.CompanyKind](company), Name: "Support workstream", Source: "manual",
			})
			if err != nil {
				t.Fatal(err)
			}
			if phase != projects.PhaseInitiative {
				_, err = e.Projects.AdvanceProjectPhase(e.Admin(), ids.From[ids.ProjectKind](ids.UUID(project.Id)), projects.AdvanceProjectPhaseInput{
					ToPhase: phase, Reason: new("Work is complete"),
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			want := 1
			if phase == projects.PhaseDelivering {
				want = 0
			}
			if staged := proposePass(t, e); staged != want {
				t.Fatalf("phase %s staged %d lifecycle offers, want %d", phase, staged, want)
			}
		})
	}
}
