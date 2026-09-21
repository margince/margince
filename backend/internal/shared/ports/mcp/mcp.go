// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mcp defines the governed tool contract (interfaces.md §2,
// 03b Layer 1). A tool registers a name, the Passport scope it requires,
// its autonomy tier, and JSON-schema in/out bound to a crm.yaml operation.
// The registry's admission gate — scope ∧ tier (∧ the read/full seat
// ceiling) — runs BEFORE Handle, so per-call policy lives in the typed
// model, never ad-hoc inside a handler. (Per-agent volume budget is
// specified but not yet enforced here; it joins this gate when the budget
// layer lands.)
package mcp

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Tool is a single governed MCP tool, exposed identically to every
// compliant agent (BYO Claude/Cursor/Copilot or the first-party runner).
type Tool interface {
	Spec() ToolSpec
	// Handle runs only after admission. Validation is the handler's typed decode,
	// NOT a check against Spec().InputSchema: the schemas are client-facing
	// documentation, so a rule that must actually hold belongs in the decode.
	Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error)
}

// UnitScopedTool is the optional half of Tool naming WHICH extension unit
// shipped the handler, so a type assertion is the one honest answer to "did this
// capability come from outside the core tree?".
//
// It states a FACT and carries no policy, which is why it lives beside Tool
// rather than in either consumer: composition and the tool registry need the
// same answer for different reasons, and a second spelling would drift. A tool
// that cannot name a unit is owned by NO unit — fail-closed for both.
//
// A string, not the SDK's extension.Name: making this port depend on the
// extension SDK would invert what the core surface is defined in terms of.
type UnitScopedTool interface{ OwningUnit() string }

// ReplayGrant names the object and action a record-less replay re-proves.
type ReplayGrant struct {
	Object string
	Action principal.Action
}

// ToolSpec is the registration shape, versioned and contract-bound to
// crm.yaml (one source of truth for wire shape).
type ToolSpec struct {
	Name string
	// The display name a client shows in place of Name. Written, not derived:
	// un-snake-casing the identifier gives back the identifier, which is what
	// this field exists to improve on.
	Title string
	// What the tool is FOR in the caller's terms: the outcome it produces, the
	// limits on producing it, when a neighbour is the better call, and what to
	// keep for the follow-up. Governance is already answered by the fields below
	// and appended by each serving surface, so a description restating it would
	// explain policing to a model whose question is which tool to call.
	Description   string
	Version       string
	RequiredScope principal.Scope
	// Marks a tool answering who the CALLER is, which every passport may ask
	// whatever else it is scoped to do. It does not relax RequiredScope and must
	// not, since ReadOnly derives from that field; this is the separate question
	// of whether the scope model applies at all, and for the caller's own
	// identity it does not.
	//
	// Without it a write-only passport is offered the writing tools and
	// refused the identity they tell it to consult, so an instruction meant
	// to remove a guess costs a failed call and then the guess anyway.
	SelfDescribing bool
	// ReplayGrant is the object grant a REPLAY re-checks, for a tool whose
	// answer names no record.
	//
	// The replay path re-reads every record a recorded answer names, because
	// object RBAC and row scope live inside the handler and a replay never
	// enters it — so a named record is normally the only authority it can
	// re-check, and an answer naming none is refused.
	//
	// A write to VOCABULARY names no record: a tag is a word, not a row with a
	// scope. For those the object grant IS what the handler checked, so it is
	// what the replay checks. Nil leaves the refusal in place, which stays
	// correct for an answer whose authority cannot be re-established at all —
	// record-less is not the same as authority-less, and this field is what
	// tells the two apart.
	ReplayGrant *ReplayGrant
	Tier        RiskTier
	// TierResolver is non-nil iff Tier == TierDynamic; the admission gate
	// calls it with the validated args plus the resolved pipeline context.
	TierResolver TierResolver
	// InputSchema/OutputSchema are client-facing documentation advertised by
	// tools/list; they are NOT generically validated (M2) — handlers decode
	// into strict typed structs and enforce their own invariants.
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	OpenAPIOp    string // the crm.yaml operationId (or logical op family) this maps to
	Egress       bool   // true if the tool reaches outside the workspace (send_email, webhooks)
	// HumanOnly marks a tool that exists in the registry so REST can dispatch
	// it by name, but that no Agent or Buyer principal may ever invoke or be
	// shown — the wire twin of a contract's x-agent-access: human-only. The
	// zero value (false) is every tool exactly as it behaves today: Tier and
	// RequiredScope are meaningless on a HumanOnly spec and must be left at
	// their zero value by whatever registers one.
	HumanOnly bool
	// UI names the interactive view that renders this tool's result, and is
	// nil on a tool that has none. It carries no authority: a view is a second
	// renderer for an answer this tool already gives in text, never a second
	// door onto the record. See ToolUI.
	UI *ToolUI
}

// ReadOnly reports whether the tool only reads — the protocol's
// readOnlyHint.
//
// DERIVED from RequiredScope rather than declared, because the scope model
// already answers this question and the gate already enforces that answer. A
// second, hand-set copy could disagree with the scope actually enforced, and
// the hint is the half that would be believed.
//
// ScopeRead and nothing else. Draft is NOT read-only: the scope covers both
// draft_email, which returns a proposal and writes nothing, and
// draft_follow_ups_for, which persists a draft activity on the deal's
// timeline through the same provider write path every other tool rides. One
// scope, two behaviours — so the scope cannot answer the question for it, and
// the conservative half is the only honest one. Reporting a tool that writes
// as read-only is a false claim; reporting draft_email as writing costs it a
// hint the protocol defaults to false anyway.
func (s ToolSpec) ReadOnly() bool {
	return s.RequiredScope == principal.ScopeRead
}

// The argument names the SURFACE owns rather than any one tool.
//
// Here in the port because two packages need them and neither owns the other:
// the tool surface splices and pops them, and the runner's listing renderer
// omits what the surface states once instead of printing it per tool. They were
// spelled in the surface alone, which left the renderer either importing the
// surface for a string or keeping a second copy of it — and a second copy of an
// argument NAME is the one that goes wrong silently, because a listing that
// omits `idempotency_ke` still renders and still looks right.
//
// ReservedApprovalIDArg has ONE production reader — the surface's own alias —
// and is here because the two are the surface's reserved pair and a reader
// looking for one should find the other beside it. What holds it to that and
// stops it drifting into the frame is the runner's assertion that the frame
// states nothing about a member the listing still renders per tool.
const (
	ReservedApprovalIDArg     = "approval_id"
	ReservedIdempotencyKeyArg = "idempotency_key"
)

// ReservedIdempotencyKeyRule is what the retry key MEANS.
//
// Held by: TestTheCompactionStillRemovesWhatTheFrameStatesOnce
// (backend/internal/compose/agenttoollistingcompaction_test.go), which reads the
// SERVED description off the real catalog and fails when it stops carrying this
// constant — so the catalogue and the frame cannot come to state the rule from
// two sources again.
//
// Two surfaces state it and they must not drift: the tool catalogue carries it
// as the member's own `description` (the surface splices it into every mutating
// core tool), and the agent runner's system frame states it once for the whole
// listing because the listing omits it per tool. It was written out twice, and
// every check on it matched the literal text — so a reword would have left the
// frame asserting a rule the catalogue no longer made, with the leak detector
// searching for a string that was no longer anywhere and passing on nothing.
//
// It reads as a bare clause rather than a sentence so each surface can frame it
// in its own voice: the schema prefixes "Optional.", the frame names the
// argument and its type first.
const ReservedIdempotencyKeyRule = "Same key, same result; a key reused with other arguments is refused."

// RiskTier is the autonomy class (A34/ADR-0026). AutoExecute and ConfirmationRequired are
// static — the declared value is the tool's whole tier. Dynamic means the
// effective tier depends on the call's arguments and MUST carry a
// TierResolver (today only advance_deal/progress_deal: 🟢 open→open,
// 🟡 to won/lost).
type RiskTier int

// The three autonomy tiers. AutoExecute and ConfirmationRequired are the
// static values; Dynamic resolves to one of them per call (see RiskTier).
const (
	TierAutoExecute RiskTier = iota
	TierConfirmationRequired
	TierDynamic
)

// TierResolver maps one call to its effective static tier. The input
// carries the validated args plus the target stage and pipeline, because
// won/lost is a property of the stage's semantic, not of the request
// arguments (a custom pipeline's renamed "Won" stage still resolves 🟡).
// Invariant: a resolver may only ever RAISE to TierConfirmationRequired — it never
// returns TierDynamic and never resolves an always-🟡 floor case to the auto-execute tier.
type TierResolver func(in TierResolverInput) RiskTier

// TierResolverInput is what the admission gate hands a resolver.
type TierResolverInput struct {
	Args json.RawMessage
	// SourceStageSemantic is the resolved semantic of the stage the record is
	// currently IN, and TargetStageSemantic the one it moves to: "open" |
	// "won" | "lost".
	//
	// Both endpoints, because the risk is not a property of the destination.
	// Moving a won deal back to an open stage is a reopen — it clears the close
	// date, the lost reason and the FX rate frozen at close, and takes revenue
	// out of a reported quarter — and a resolver shown only the target sees an
	// ordinary open-stage move.
	SourceStageSemantic string
	TargetStageSemantic string
	PipelineID          string
	// ObservedVersion is the version of the record the tier decision was read
	// from, and it is what binds that decision to the write it admits.
	//
	// A dynamic tier is resolved from a record's state, and that read commits
	// before the write does. Without the version, an auto-execute answer means
	// only "the record permitted this when the gate looked" — the record can
	// change in the window, and the agent controls both sides of it. The
	// admission point carries this into the write as its precondition, so a
	// record that moved in between loses to the version compare instead of to
	// timing.
	//
	// nil means the tool's tier did not turn on a record's state, so there is
	// nothing to bind.
	ObservedVersion *int64
}

// Registry admits and dispatches tools. Registration is init()-time,
// one file per tool; a duplicate name fails fast at boot.
type Registry interface {
	Register(t Tool)
	// Invoke runs the admission gate (scope ∧ tier ∧ seat ceiling) and then
	// the tool. A 🟡 call without a valid approval token returns
	// apperrors.ErrRequiresApproval with the staged approval reference.
	Invoke(ctx context.Context, name string, in json.RawMessage) (json.RawMessage, error)
	Specs() []ToolSpec
}
