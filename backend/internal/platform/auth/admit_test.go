// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// stubAuthority is the seam double: the gate must take seat + RBAC from
// HERE (the live read), never from whatever the transport stamped on the
// principal.
type stubAuthority struct {
	seat principal.SeatType
	rbac authz.RBAC
	err  error
	// passportGone stands for the credential being killed while a run is in
	// flight: the seam answers not-found for a revoked, expired or re-granted
	// passport, and the gate has to refuse on it.
	passportGone bool
	// passports records what the gate asked about, so a case can hold that it
	// asked about the RIGHT credential rather than merely asking.
	passports []ids.UUID
	reads     int
}

func (s *stubAuthority) EffectiveRBAC(ctx context.Context, ws, human ids.UUID) (authz.RBAC, error) {
	s.reads++
	return s.rbac, s.err
}

func (s *stubAuthority) SeatType(ctx context.Context, ws, human ids.UUID) (principal.SeatType, error) {
	s.reads++
	return s.seat, s.err
}

var (
	testWorkspace = ids.NewV7()
	testHuman     = ids.NewV7()
)

func fullSeatGate() *Gate { return NewGate(&stubAuthority{seat: principal.SeatFull}) }

func agentCtx(scopes ...principal.Scope) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), testWorkspace)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test",
		OnBehalfOf: testHuman,
		Scopes:     principal.NewScopeSet(scopes...),
	})
}

func noResolve() (mcp.TierResolverInput, error) { return mcp.TierResolverInput{}, nil }

func TestScopeIsCheckedBeforeTier(t *testing.T) {
	authority := &stubAuthority{seat: principal.SeatFull}
	spec := mcp.ToolSpec{Name: "send_email", RequiredScope: principal.ScopeSend, Tier: mcp.TierConfirmationRequired}

	_, err := NewGate(authority).Admit(agentCtx(principal.ScopeRead, principal.ScopeWrite), spec, noResolve)
	if !errors.Is(err, apperrors.ErrScopeExceeded) {
		t.Fatalf("missing scope → %v, want ErrScopeExceeded (never ErrRequiresApproval: an out-of-scope caller must not learn the tier)", err)
	}
	if authority.reads != 0 {
		t.Fatal("an out-of-scope call must not cost an authority read")
	}
}

func TestConfirmationRequiredIsBlockedBehindApproval(t *testing.T) {
	spec := mcp.ToolSpec{Name: "archive_record", RequiredScope: principal.ScopeWrite, Tier: mcp.TierConfirmationRequired}
	if _, err := fullSeatGate().Admit(agentCtx(principal.ScopeWrite), spec, noResolve); !errors.Is(err, apperrors.ErrRequiresApproval) {
		t.Fatalf("🟡 without approval → %v, want ErrRequiresApproval", err)
	}
}

func TestAutoExecuteIsAdmitted(t *testing.T) {
	spec := mcp.ToolSpec{Name: "read_record", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute}
	if _, err := fullSeatGate().Admit(agentCtx(principal.ScopeRead), spec, noResolve); err != nil {
		t.Fatalf("🟢 in scope → %v, want admitted", err)
	}
}

func TestDynamicTierIsResolvedPerCall(t *testing.T) {
	resolver := func(in mcp.TierResolverInput) mcp.RiskTier {
		if in.TargetStageSemantic == "open" {
			return mcp.TierAutoExecute
		}
		return mcp.TierConfirmationRequired
	}
	spec := mcp.ToolSpec{
		Name: "advance_deal", RequiredScope: principal.ScopeWrite,
		Tier: mcp.TierDynamic, TierResolver: resolver,
	}

	// The 🟢 arm names the version its record was read at, because an
	// auto-executed dynamic call that names none is raised rather than run —
	// see TestADynamicTierThatNamesNoRecordVersionIsRaisedRatherThanRun.
	open := func() (mcp.TierResolverInput, error) {
		return mcp.TierResolverInput{TargetStageSemantic: "open", ObservedVersion: version(1)}, nil
	}
	if _, err := fullSeatGate().Admit(agentCtx(principal.ScopeWrite), spec, open); err != nil {
		t.Fatalf("open→open resolves 🟢: %v", err)
	}

	won := func() (mcp.TierResolverInput, error) {
		return mcp.TierResolverInput{TargetStageSemantic: "won"}, nil
	}
	if _, err := fullSeatGate().Admit(agentCtx(principal.ScopeWrite), spec, won); !errors.Is(err, apperrors.ErrRequiresApproval) {
		t.Fatalf("move to won → %v, want ErrRequiresApproval (the always-🟡 floor)", err)
	}
}

func TestDynamicWithoutResolverFailsClosed(t *testing.T) {
	spec := mcp.ToolSpec{Name: "broken", RequiredScope: principal.ScopeWrite, Tier: mcp.TierDynamic}
	_, err := fullSeatGate().Admit(agentCtx(principal.ScopeWrite), spec, noResolve)
	if err == nil || errors.Is(err, apperrors.ErrRequiresApproval) {
		t.Fatalf("mis-registered dynamic spec → %v, want a hard failure, not a tier decision", err)
	}
}

func TestHumansDoNotRideTheScopeModel(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:u1",
	})
	spec := mcp.ToolSpec{Name: "archive_record", RequiredScope: principal.ScopeWrite, Tier: mcp.TierConfirmationRequired}
	if _, err := NewGate(nil).Admit(ctx, spec, noResolve); err != nil {
		t.Fatalf("human through the gate → %v; human authority is their RBAC, enforced at the store", err)
	}
}

func TestSeatIsReDerivedLiveAtAdmission(t *testing.T) {
	// The transport stamped a FULL seat, but the live read says the
	// granting human is now a read seat: the live answer wins — a seat
	// downgrade binds mid-session (RT-AR-M11), refused with the seat
	// sentinel BEFORE the tier is consulted and never staged (no
	// approval lifts a licensing ceiling, A62/ADR-0047).
	gate := NewGate(&stubAuthority{seat: principal.SeatRead})
	ctx := principal.WithWorkspaceID(context.Background(), testWorkspace)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test",
		OnBehalfOf: testHuman, SeatType: principal.SeatFull,
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
	})

	write := mcp.ToolSpec{Name: "create_record", RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute}
	_, err := gate.Admit(ctx, write, noResolve)
	if !errors.Is(err, apperrors.ErrSeatTierInsufficient) {
		t.Fatalf("live read seat under a stamped full seat → %v, want ErrSeatTierInsufficient", err)
	}
	if errors.Is(err, apperrors.ErrRequiresApproval) {
		t.Fatal("a read-seat mutation must not be staged for approval")
	}

	read := mcp.ToolSpec{Name: "read_record", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute}
	if _, err := gate.Admit(ctx, read, noResolve); err != nil {
		t.Fatalf("read-seat read tool → %v, want admitted", err)
	}
}

func TestAdmissionRefreshesThePrincipalAuthority(t *testing.T) {
	// The admitted context carries the re-derived grants, so store-level
	// RBAC downstream runs on current authority, not the stamped copy.
	livePerms := principal.Permissions{
		RoleKeys: []string{"sales"},
		Objects:  map[string]principal.ObjectGrant{"person": {Create: true, Read: true}},
		RowScope: principal.RowScopeTeam,
	}
	gate := NewGate(&stubAuthority{seat: principal.SeatFull, rbac: authz.RBAC{Permissions: livePerms}})
	spec := mcp.ToolSpec{Name: "create_record", RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute}

	ctx, err := gate.Admit(agentCtx(principal.ScopeWrite), spec, noResolve)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	p, _ := principal.Actor(ctx)
	if !p.Permissions.Allows("person", principal.ActionCreate) || p.Permissions.RowScope != principal.RowScopeTeam {
		t.Fatalf("admitted principal carries %+v, want the live-resolved grants", p.Permissions)
	}
}

func TestUnresolvableGrantingHumanIsDenied(t *testing.T) {
	// A revoked/archived granting human resolves ErrNotFound in the seam;
	// the gate answers denial, and an infrastructure failure stays an
	// error — an outage is never an authorization answer.
	gone := NewGate(&stubAuthority{err: apperrors.ErrNotFound})
	spec := mcp.ToolSpec{Name: "create_record", RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute}
	if _, err := gone.Admit(agentCtx(principal.ScopeWrite), spec, noResolve); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("vanished granting human → %v, want ErrPermissionDenied", err)
	}

	down := NewGate(&stubAuthority{err: errors.New("connection refused")})
	if _, err := down.Admit(agentCtx(principal.ScopeWrite), spec, noResolve); errors.Is(err, apperrors.ErrPermissionDenied) || err == nil {
		t.Fatalf("resolver outage → %v, want a hard error, not a policy answer", err)
	}
}

func TestUncomposedGateFailsClosedForAgents(t *testing.T) {
	// A gate composed without an authority resolver (and a nil gate)
	// must refuse agents rather than admit on missing data.
	spec := mcp.ToolSpec{Name: "create_record", RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute}
	if _, err := NewGate(nil).Admit(agentCtx(principal.ScopeWrite), spec, noResolve); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("resolver-less gate → %v, want ErrPermissionDenied", err)
	}
	var nilGate *Gate
	if _, err := nilGate.Admit(agentCtx(principal.ScopeWrite), spec, noResolve); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("nil gate → %v, want ErrPermissionDenied", err)
	}
}

func TestAgentWithoutGrantingHumanIsDenied(t *testing.T) {
	// OnBehalfOf is the authority key; an agent principal without one (or
	// outside any workspace) has no derivable authority.
	ctx := principal.WithWorkspaceID(context.Background(), testWorkspace)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test",
		Scopes: principal.NewScopeSet(principal.ScopeWrite),
	})
	spec := mcp.ToolSpec{Name: "create_record", RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute}
	if _, err := fullSeatGate().Admit(ctx, spec, noResolve); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("agent without granting human → %v, want ErrPermissionDenied", err)
	}
}

func TestNoPrincipalIsRefused(t *testing.T) {
	spec := mcp.ToolSpec{Name: "read_record", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute}
	if _, err := fullSeatGate().Admit(context.Background(), spec, noResolve); err == nil {
		t.Fatal("anonymous context admitted")
	}
}

// AdmittedAuthority is the one read the gate makes. passportGone is what a
// revoked, expired or re-granted credential answers with — the seam's own
// contract — so the gate's behaviour on it can be driven without a database.
func (s *stubAuthority) AdmittedAuthority(_ context.Context, _, _, passport ids.UUID) (authz.RBAC, principal.SeatType, error) {
	s.reads++
	s.passports = append(s.passports, passport)
	if s.passportGone {
		return authz.RBAC{}, "", apperrors.ErrNotFound
	}
	return s.rbac, s.seat, s.err
}

// TestARevokedPassportIsRefusedAtTheNextToolCall — the gate acting on the
// seam's refusal, which is where a revoked credential actually stops.
//
// The run authenticated once and holds a principal that still reads as valid:
// the scopes are there, the human's seat is full, nothing about the cached
// value says the credential is dead. Only the re-read does.
func TestARevokedPassportIsRefusedAtTheNextToolCall(t *testing.T) {
	authority := &stubAuthority{seat: principal.SeatFull, passportGone: true}
	spec := mcp.ToolSpec{Name: "list_people", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute}

	_, err := NewGate(authority).Admit(agentCtx(principal.ScopeRead), spec, noResolve)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a tool call on a revoked passport answered %v, want permission denied — the killed credential is still executing", err)
	}
	if authority.reads == 0 {
		t.Error("the gate admitted without asking the seam at all")
	}
}

// TestTheGateAsksAboutThePrincipalsOwnPassport — asking is not enough; it has
// to ask about the credential this call is riding.
//
// A gate that passed a zero id would re-check nothing while looking exactly
// like a gate that re-checks, and the seam reads a zero as "this principal
// holds no credential" — which is the one answer that must not be inferred
// from a bug.
func TestTheGateAsksAboutThePrincipalsOwnPassport(t *testing.T) {
	authority := &stubAuthority{seat: principal.SeatFull}
	spec := mcp.ToolSpec{Name: "list_people", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute}
	passport := ids.NewV7()

	ctx := principal.WithWorkspaceID(context.Background(), testWorkspace)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test",
		OnBehalfOf: testHuman, PassportID: passport,
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})
	if _, err := NewGate(authority).Admit(ctx, spec, noResolve); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if len(authority.passports) != 1 || authority.passports[0] != passport {
		t.Errorf("the gate asked about %v, want the principal's own passport %s", authority.passports, passport)
	}
}
