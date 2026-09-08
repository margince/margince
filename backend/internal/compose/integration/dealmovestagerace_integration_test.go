// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The window the deal's own version pin cannot see.
//
// The tier gate auto-executes a deal move exactly when it can prove BOTH
// endpoints open, and it proves that by reading two STAGE rows. The pin it
// carries forward binds the DEAL — and a stage row is mutable independently of
// any deal, so an admin changing the target stage's semantic in between leaves
// the pin perfectly satisfied while the move being made is no longer the move
// that was admitted.
//
// The racing actor is a human admin rather than the agent: pipeline and stage
// configuration is human-only, so an agent cannot arrange either side of this.
// That is what makes it a different defect from the one the pin was added for,
// and why the pin is not the fix.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func TestAnAdmittedMoveRefusesAStageThatClosedAfterTheGateReadIt(t *testing.T) {
	e := Setup(t)
	pipeline, open, target := DealFixture(t, e)
	deal := e.SeedDeal(t, "Stage race", pipeline, open, &e.Rep1)
	// The target starts OPEN, which is the config the gate is about to read.
	e.WsExec(t, `UPDATE stage SET semantic = 'open' WHERE id = $1`, target)
	store := deals.NewStore(e.DB(), installseam.Deals())

	// The gate's verdict, taken while BOTH stages are open — which is the state
	// that makes this move auto-executable in the first place.
	admitted := admitOpenToOpenMove(t, e)

	// Then the admin acts, in the window. Nothing about the deal changes, so
	// its version — the whole of what the gate pinned — still names the row.
	e.WsExec(t, `UPDATE stage SET semantic = 'won' WHERE id = $1`, target)

	_, err := store.AdvanceDeal(admitted, ids.From[ids.DealKind](deal), deals.AdvanceDealInput{
		ToStageID:                target,
		WonWithoutContractReason: wonReasonImported(),
	})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("err = %v, want ErrVersionSkew — the move was admitted unattended as open-to-open "+
			"and closed the deal instead", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE id = $1 AND status = 'open' AND closed_at IS NULL`, deal); n != 1 {
		t.Error("the deal was closed by a move the gate never admitted")
	}
}

// The same call with the stage config unchanged still lands: the re-check is a
// guard on a premise that moved, not a second approval gate.
func TestAnAdmittedMoveStillLandsWhenNoStageChanged(t *testing.T) {
	e := Setup(t)
	pipeline, open, target := DealFixture(t, e)
	deal := e.SeedDeal(t, "Stage race control", pipeline, open, &e.Rep1)
	e.WsExec(t, `UPDATE stage SET semantic = 'open' WHERE id = $1`, target)
	store := deals.NewStore(e.DB(), installseam.Deals())

	admitted := admitOpenToOpenMove(t, e)
	moved, err := store.AdvanceDeal(admitted, ids.From[ids.DealKind](deal), deals.AdvanceDealInput{
		ToStageID: target,
	})
	if err != nil {
		t.Fatalf("a routine open-to-open move was refused: %v", err)
	}
	if moved.StageId == nil || ids.UUID(*moved.StageId) != target.UUID {
		t.Errorf("the deal sits in %v, want the stage the move named", moved.StageId)
	}
}

// A move by a HUMAN is unaffected: the tier model does not govern one, so there
// is no admitted premise to re-check and a stage change is simply the config
// they are moving under.
func TestAHumansMoveIsNotHeldToTheGatesPremise(t *testing.T) {
	e := Setup(t)
	pipeline, open, target := DealFixture(t, e)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms)
	deal := e.SeedDeal(t, "Human move", pipeline, open, &e.Rep1)
	store := deals.NewStore(e.DB(), installseam.Deals())

	if _, err := store.AdvanceDeal(human, ids.From[ids.DealKind](deal), deals.AdvanceDealInput{
		ToStageID:                target,
		WonWithoutContractReason: wonReasonImported(),
	}); err != nil {
		t.Fatalf("a human's move onto a stage that is now won was refused: %v", err)
	}
}

// wonReasonImported is the reason a win with no agreement behind it carries;
// the two closing cases here are about the stage config, not about evidence.
func wonReasonImported() *string {
	reason := "imported"
	return &reason
}

// admitOpenToOpenMove answers a context carrying the gate's own auto-execute
// verdict for an open-to-open move.
//
// Through auth.Gate.Admit rather than by minting the pin: the marker is
// deliberately unexported, so a caller able to mint one could condition its own
// write on a premise nothing ever established — and a test that minted it would
// be asserting against its own fixture rather than against the gate.
func admitOpenToOpenMove(t *testing.T, e *Env) context.Context {
	t.Helper()
	spec := mcp.ToolSpec{
		Name: "advance_deal", RequiredScope: principal.ScopeWrite, Tier: mcp.TierDynamic,
		TierResolver: func(in mcp.TierResolverInput) mcp.RiskTier {
			if in.SourceStageSemantic == "open" && in.TargetStageSemantic == "open" {
				return mcp.TierAutoExecute
			}
			return mcp.TierConfirmationRequired
		},
	}
	version := int64(1)
	resolve := func() (mcp.TierResolverInput, error) {
		return mcp.TierResolverInput{
			SourceStageSemantic: "open", TargetStageSemantic: "open", ObservedVersion: &version,
		}, nil
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	// An agent principal, because the tier model governs one.
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:stage-race",
		UserID: e.AdminUser, OnBehalfOf: e.AdminUser, TeamIDs: []ids.UUID{e.Team1},
		Scopes: principal.NewScopeSet(principal.ScopeWrite),
	})
	admitted, err := auth.NewGate(fullAuthority{}).Admit(ctx, spec, resolve)
	if err != nil {
		t.Fatalf("the gate refused a routine open-to-open move: %v", err)
	}
	if _, ok := auth.AutoExecutePin(admitted); !ok {
		t.Fatal("the admitted context carries no auto-execute pin, so this case would prove nothing")
	}
	return admitted
}

// fullAuthority is the identity seam stubbed to a full seat holding the deal
// grants.
//
// The seam and not the store: Admit RE-DERIVES the granting human's authority
// and overwrites whatever the principal carried, so a real resolver would make
// this case turn on how the harness seeds roles rather than on the premise it is
// about — and it would fail for a reason with nothing to do with stage config.
// The resolver is a true boundary (identity), which is what makes stubbing it
// the right call rather than a shortcut.
type fullAuthority struct{}

func (fullAuthority) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: AdminPerms}, nil
}

func (fullAuthority) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	return principal.SeatFull, nil
}

func (fullAuthority) AdmittedAuthority(context.Context, ids.UUID, ids.UUID, ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authz.RBAC{Permissions: AdminPerms}, principal.SeatFull, nil
}
