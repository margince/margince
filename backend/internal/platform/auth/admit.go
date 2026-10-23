// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package auth is the ONE admission point for governed agent actions
// (interfaces.md §2, ADR-0055): scope ∧ seat ∧ tier ∧ volume, resolved against
// the calling Principal, BEFORE any handler runs — whether the action arrives
// as an MCP tool call or a mutating REST operation. It is its own package
// so nothing else can mint an admitted capability — Surface A (inbound
// agents) and Surface B (our own runner) both enter here, and there is no
// other door.
package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Gate admits governed actions. Its authority source is the
// shared/ports/authz seam: identity implements it, the composition root
// injects it, and Admit re-derives the granting human's seat + RBAC live
// at every admission — so a revocation binds mid-session (RT-AR-M11)
// instead of surviving on whatever the transport stamped earlier.
type Gate struct {
	authority authz.Resolver
	volume    VolumeMeter
}

// GateOption configures a Gate at construction. Variadic rather than
// positional so the six existing composition sites are not rewritten every
// time the gate grows a dependency, which is how one of them ends up passing
// nil by copy-paste.
type GateOption func(*Gate)

// WithVolumeMeter installs the per-Passport volume meter (MCP-SESS-*). A gate built
// WITHOUT one does not enforce any of those bounds, which is a real composition
// rather than an oversight: the Surface-B runner and the workflow paths build a
// gate and run as the human or system that started them, whom these bounds do
// not govern. The api server — the one role an agent principal ever reaches —
// passes it, from the same pointer it gives the registry that charges it.
func WithVolumeMeter(quota VolumeMeter) GateOption {
	return func(g *Gate) { g.volume = quota }
}

// NewGate builds the admission point over its authority seam. Options add
// the dependencies only some roles have — today, the volume meter.
func NewGate(authority authz.Resolver, opts ...GateOption) *Gate {
	g := &Gate{authority: authority}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Admit decides whether the context's principal may run the action with
// spec's tier model. resolve supplies the TierResolverInput for dynamic
// tools. It is called lazily, only when the spec is TierDynamic, because
// building it may cost a database read (the target stage's semantic).
//
// The decision order matters. Scope comes first, so a caller without the verb
// never learns the tier and pays no authority read. Then the live seat ceiling,
// the volume ceilings and the tier. A 🟡 outcome (declared or dynamically
// resolved) returns ErrRequiresApproval. On admission the returned context
// carries the principal refreshed with the re-derived authority, so downstream
// store RBAC runs on current grants.
func (g *Gate) Admit(ctx context.Context, spec mcp.ToolSpec, resolve func() (mcp.TierResolverInput, error)) (context.Context, error) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return ctx, errors.New("gate: no principal bound to context")
	}

	// Humans and the system principal do not ride the gate's scope model:
	// their authority is their RBAC, enforced at the store. The gate
	// exists to bound AGENTS (03b Layer 1); a human reaching a tool
	// through the UI already answered for the action.
	// A buyer holds no scope, no seat and no volume bound, so "not an agent, pass"
	// would wave one through every check this gate exists to apply. Refused
	// by kind rather than by the absence of a passport, because the absence
	// is what makes the pass-through look safe.
	if p.Type == principal.PrincipalBuyer {
		return ctx, fmt.Errorf("gate: %s: a Deal Room participant holds no tool authority: %w",
			spec.Name, apperrors.ErrPermissionDenied)
	}
	if p.Type != principal.PrincipalAgent {
		return ctx, nil
	}

	// A tool that answers who the CALLER is passes the scope axis whatever the
	// passport holds — mcp.ToolSpec.SelfDescribing carries the reason, and the
	// listing filter reads the same flag, so what is offered is what is
	// admitted. Every check below still applies: the seat, the re-derived
	// RBAC and the volume bounds are not waived by knowing whose seat it is.
	if !spec.SelfDescribing && !p.Scopes.Has(spec.RequiredScope) {
		return ctx, &ScopeRequiredError{Tool: spec.Name, Scope: spec.RequiredScope}
	}

	// Re-derive the granting human's authority through the seam — never
	// trust the principal's stamped copy for an admission decision. A
	// gate composed without a resolver, or an agent without a granting
	// human, fails closed: absence of authority data is denial.
	if g == nil || g.authority == nil {
		return ctx, fmt.Errorf("gate: %s: no authority resolver composed: %w", spec.Name, apperrors.ErrPermissionDenied)
	}
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok || p.OnBehalfOf.IsZero() {
		return ctx, fmt.Errorf("gate: %s: agent principal lacks workspace or granting human: %w", spec.Name, apperrors.ErrPermissionDenied)
	}
	// ONE read, answering three questions about one instant.
	//
	// The passport's own liveness is asked HERE and not only at authentication,
	// which is the whole of RT-AR-M11 rather than half of it. A run
	// authenticates once at start and then executes for its whole wall clock,
	// so revoking a passport mid-run used to stop nothing: revocation binds "at
	// the next token lookup", and inside a run there was no next lookup. This
	// call is that lookup, on every tool.
	//
	// A principal carrying NO passport is not waved past that — it holds no
	// credential for anybody to revoke. Two production paths mint one, and they
	// answer the revocation question differently: an extension job tick
	// (compose/extjobsrun.go) holds no authority AT ALL — no user, no seat, no
	// permissions, so every object is denied to it and the records it lands go
	// through Ingest under the member's own live grants; and the auto-apply
	// actor (compose/autoapply.go), whose authority IS a record owner's plus a
	// write scope, derived from a live human at construction. Neither is a
	// long-lived token. A third one is a decision rather than an oversight, which is what
	// gates/passportlessagents_test.go keeps true.
	//
	// The seat and the grants come back together for the reason identity's own
	// EffectiveAuthority gives: read separately they can compose an authority
	// the member never held, a role change and a seat change crossing between
	// two transactions leaving permissions from before beside a seat from
	// after. Both are ceilings on the same act.
	rbac, seat, err := g.authority.AdmittedAuthority(ctx, wsID, p.OnBehalfOf, p.PassportID)
	if err != nil {
		return ctx, deniedIfGone("the credential or the human behind it", spec.Name, err)
	}
	p.SeatType, p.Permissions, p.TeamIDs = seat, rbac.Permissions, rbac.TeamIDs
	ctx = principal.WithActor(ctx, p)

	// The seat ceiling is checked before tier (A62/ADR-0047): a read seat —
	// or an agent acting for one — may run only read-scoped tools, whatever
	// their passport scope or the target's tier would otherwise permit. A
	// non-read tool is refused outright, never staged for approval, because
	// no approval can lift a licensing ceiling.
	if spec.RequiredScope != principal.ScopeRead && !seat.CanMutate() {
		return ctx, fmt.Errorf("gate: %s needs a full seat; %s acts for a read seat: %w",
			spec.Name, p.ID, apperrors.ErrSeatTierInsufficient)
	}

	// Volume, the third term of "scope ∧ tier ∧ volume" (interfaces.md §2). It
	// runs AFTER scope and seat so a caller who may not run the verb at all
	// never learns that a bound exists, let alone how much of it is spent.
	//
	// Which counters bind THIS call, and what crossing one costs, is volume.go's
	// business. It runs before tier because a suspended Passport is refused
	// whatever the tier resolves to, and resolving a dynamic tier costs a
	// database read the refused caller should not be able to make us pay.
	if err := g.refuseOnVolume(ctx, spec); err != nil {
		return ctx, err
	}

	tier, observed, err := resolveTier(spec, resolve)
	if err != nil {
		return ctx, err
	}
	// A dynamic tier that cannot name the record it was read from is RAISED, not
	// admitted. Without this the bind below is advisory: a dynamic tool that
	// answers no version would run unattended with nothing conditioning its
	// write, which is the unpinned auto-execute this whole path exists to end —
	// restored by omission rather than by decision. Raising is the shape the
	// resolver contract already takes (it may only ever raise), and a human is
	// the safe answer to "this server could not establish the record's state".
	// An agent acting as its user (principal.ActsAsItsUser) runs whatever the
	// tier says: what the colleague may do unaided, their agent may do too. The
	// tier then decides nothing, so a version it could not name raises nothing,
	// and one it did name is still bound below. Automatic work with nobody
	// behind it keeps both rules.
	resolvedAuto := tier == mcp.TierAutoExecute
	if TierFor(p, spec) == mcp.TierAutoExecute {
		tier = mcp.TierAutoExecute
	} else if tier == mcp.TierAutoExecute && spec.Tier == mcp.TierDynamic && observed == nil {
		return ctx, fmt.Errorf(
			"gate: %s resolved to auto-execute without naming the record version it was resolved from: %w",
			spec.Name, apperrors.ErrRequiresApproval)
	}
	if tier != mcp.TierAutoExecute {
		return ctx, fmt.Errorf("gate: %s is a confirm-first (🟡) action: %w", spec.Name, apperrors.ErrRequiresApproval)
	}
	// The tier said this call may run unattended, and for a dynamic tool it said
	// so by reading a record. That read commits before the write it admits, and
	// the agent controls both sides of the window, so the answer is only true of
	// the record as it WAS. Binding the version here is what makes the write
	// re-check it inside the transaction that mutates, where a record that moved
	// in between loses to the version compare instead of to timing.
	//
	// Set HERE rather than by each dispatch layer: the MCP registry and the REST
	// agent gate both come through Admit, and a rule spelled twice on this seam
	// is how the tier itself came to judge both endpoints of a deal move on one
	// door and only the destination on the other.
	// A tier lifted for an agent acting as its user binds no pin. The pin also
	// tells the store the move is the open one the tier admitted. A lifted call
	// is the colleague's own move, which the tier model does not govern.
	if observed != nil && resolvedAuto {
		ctx = withAutoExecutePin(ctx, *observed)
	}
	return ctx, nil
}

// resolveTier answers the call's effective tier, plus the version of the record
// a dynamic tier was decided from — nil when the tier turned on no record, which
// is every static tier and any dynamic tool that reports none.
func resolveTier(spec mcp.ToolSpec, resolve func() (mcp.TierResolverInput, error)) (mcp.RiskTier, *int64, error) {
	if spec.Tier != mcp.TierDynamic {
		// A static tier is the tool's whole tier: nothing was read to decide it,
		// so a resolver input built for one names no record this call proved
		// anything about.
		return spec.Tier, nil, nil
	}
	if spec.TierResolver == nil {
		// Registration should have refused this spec; failing closed
		// here keeps a mis-registered tool from defaulting to 🟢.
		return spec.Tier, nil, fmt.Errorf("gate: %s is TierDynamic without a resolver", spec.Name)
	}
	in, err := resolve()
	if err != nil {
		return spec.Tier, nil, err
	}
	return spec.TierResolver(in), in.ObservedVersion, nil
}

// autoExecutePinKey carries the version a dynamic tier was resolved from, on a
// call this gate admitted at the auto-execute tier.
type autoExecutePinKey struct{}

// withAutoExecutePin is unexported and reached from exactly one place: the 🟢
// outcome of Admit. The pin asserts that THIS gate proved THIS record's state,
// and a caller able to mint one could condition its own write on a version
// nothing ever checked.
func withAutoExecutePin(ctx context.Context, version int64) context.Context {
	return context.WithValue(ctx, autoExecutePinKey{}, version)
}

// AutoExecutePin answers the version the gate resolved this call's dynamic tier
// from, for the transports that turn it into the write's precondition — an
// `if_version` argument on the MCP door, an `If-Match` header on the REST one.
//
// ok is false for every call whose tier was not decided by reading a record: a
// static tier, a tier raised to confirm-first (whose pin is the approval's,
// taken at the moment the human was shown the record), and a human principal,
// whom the gate's tier model does not govern.
func AutoExecutePin(ctx context.Context) (version int64, ok bool) {
	version, ok = ctx.Value(autoExecutePinKey{}).(int64)
	return version, ok
}

// deniedIfGone turns the seam's ABSENCE answer into a denial, and lets
// infrastructure failures pass through — so an outage reads as an error and
// never as an authorization answer.
//
// WHAT is the caller's word for the thing that could not be resolved, and the
// message takes it rather than supplying one. The admission read folds four
// ways to answer not-found into a single call — a revoked passport, an expired
// one, one re-granted to somebody else, and a human who is gone — so naming
// the human for all four sends an operator to look at the account when they
// killed the credential a moment ago.
func deniedIfGone(what, tool string, err error) error {
	if errors.Is(err, apperrors.ErrNotFound) {
		return fmt.Errorf("gate: %s: %s no longer resolvable: %w", tool, what, apperrors.ErrPermissionDenied)
	}
	return fmt.Errorf("gate: %s: resolving %s: %w", tool, what, err)
}

// AdmitRead applies the calls ceiling alone, for a call that has no tool spec to
// admit against. It is the non-mutating half of the REST surface.
//
// That path has no tier to resolve and no scope to check against a spec. A GET's
// authority is the granting human's RBAC at the store. It spends the same ceiling
// a tool call does. A read has no counter of its own, so one request is one call
// on either door. A Passport suspended through MCP cannot keep reading through
// /v1.
//
// This half only refuses. The caller charges the call once it is admitted
// (modules/agents ChargeAdmittedRead), because a gate that could both admit and
// charge could admit itself.
//
// Non-agents are admitted untouched, as Admit leaves them.
func (g *Gate) AdmitRead(ctx context.Context) error {
	if g == nil || g.volume == nil {
		return nil
	}
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalAgent {
		return nil
	}
	if reading := g.volume.Read(ctx, agentvolume.Calls); reading.Exceeded {
		return &VolumeExceededError{Reading: reading}
	}
	return nil
}

// TierFor is the tier spec carries for p before any argument is read. An agent
// acting as its user (principal.ActsAsItsUser) runs every tool as the colleague
// could, so it sees auto-execute, except where spec.WaitsEvenForItsUser. The
// gate admits on it, and the tool listing advertises it, so what a caller is
// told is what it is admitted on.
func TierFor(p principal.Principal, spec mcp.ToolSpec) mcp.RiskTier {
	if p.ActsAsItsUser() && !spec.WaitsEvenForItsUser {
		return mcp.TierAutoExecute
	}
	return spec.Tier
}
