// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package workflow defines the automation seam (interfaces.md §5,
// features/03 §5): workflows are typed handlers in a registry — code,
// agent-authored, test-guarded — not a visual builder. Each declares a
// trigger, a typed Effect, an idempotency key, and a risk tier; runs ride
// the job queue with retries, dead-letter, and audit.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Handler is the seam an agent implements to add automation. Registered
// by Spec().Name; subscribed by Spec().Trigger.
type Handler interface {
	Spec() Spec

	// Match is a pure predicate over the trigger event and related
	// records; false means the handler does not run.
	Match(ctx context.Context, ev Event) (bool, error)

	// Plan computes the typed Effect WITHOUT applying it — this is what
	// makes dry-run and diff preview possible. Deterministic given the
	// same event and DB snapshot.
	Plan(ctx context.Context, ev Event) (Effect, error)

	// Apply executes the planned Effect. 🟢 effects auto-execute; a 🟡
	// effect must carry an approval token or Apply returns
	// apperrors.ErrRequiresApproval. Idempotent on IdempotencyKey(ev).
	Apply(ctx context.Context, ev Event, eff Effect, token *ApprovalToken) (RunResult, error)

	// IdempotencyKey derives the stable key for this (handler, event) so
	// the queue and registry dedupe replays.
	IdempotencyKey(ev Event) string
}

type Spec struct {
	Name    string // stable id: "flag_idle_deals", "route_lead", …
	Trigger Trigger
	Tier    mcp.RiskTier
}

// Trigger binds to the event bus or a schedule: EventType for bus events,
// Schedule when EventType is empty — a clock:<name> marker, never a cron
// expression (modules/automation/handlers_clock.go's
// noActivityScheduleMarker doc): the real cadence is the River periodic
// job's own interval, and a Schedule-bearing handler also needs its own
// candidate source wired at the time-scan (modules/automation/
// timescan.go's activityScanHandlers is the only wired source today) or
// it registers but is never actually evaluated.
type Trigger struct {
	EventType string
	Schedule  string
	Filter    map[string]any // cheap envelope pre-filter before Match
}

// Event is the bus envelope slice a handler sees (events.md §2), plus
// the automation instance driving this dispatch: the engine fires a
// handler once per enabled instance of its type, carrying that
// instance's validated params — the editor's parameterization reaches
// the run here.
type Event struct {
	ID          ids.UUID
	Type        string
	WorkspaceID ids.UUID
	OccurredAt  time.Time
	Entity      datasource.EntityRef
	Payload     json.RawMessage

	AutomationID ids.UUID
	Params       json.RawMessage

	// OwnerID is the human who authored this automation instance
	// (automation.owner_id) — the "on behalf of" attribute of a firing,
	// analogous to AutomationID. Zero for a system-seeded automation (no
	// owner_id was ever stamped) and for a system handler (no instance
	// behind it at all): the match-time owner-permission gate reads this
	// to decide whose live authority a firing must still hold.
	OwnerID ids.UUID
}

// Effect is the typed, enumerable set of actions a run may take. No
// free-form side effects: each action is a declared variant so dry-run,
// audit, and the 🟡 gate can reason about it.
type Effect struct {
	Actions []Action

	// Handler and OccurrenceKey scope the effect-level idempotency claim
	// the create executor takes before writing (automation's applyCreate):
	// N enabled instances of one handler each dispatch off the same
	// occurrence, and an IDENTICAL planned create must apply once across
	// all of them — the per-instance run claim cannot see that, because
	// its key carries the automation id. OccurrenceKey is the handler's
	// own IdempotencyKey(ev) WITHOUT the instance suffix: for an event
	// trigger that carries the bus event id (one delivery, one key), and
	// for a clock trigger the anchor-derived occurrence key — the scan
	// synthesizes a fresh event id per instance pass, so an event-id key
	// would silently give every instance its own claim and no dedupe at
	// all. The engine stamps both just before Apply; a caller applying an
	// effect outside the engine leaves Handler empty and applies
	// unclaimed, which is the pre-existing single-caller contract.
	Handler       string
	OccurrenceKey string
}

// ActionKind enumerates the closed action set (features/03 §5.1); the
// closed-set contract test is the anti-builder guard.
type ActionKind string

const (
	ActionCreateRecord   ActionKind = "create_record"
	ActionUpdateRecord   ActionKind = "update_record"
	ActionCreateTask     ActionKind = "create_task"
	ActionAssignOwner    ActionKind = "assign_owner"
	ActionAdvanceDeal    ActionKind = "advance_deal"
	ActionSendEmail      ActionKind = "send_email"
	ActionEmitFlowEvent  ActionKind = "emit_flow_event"
	ActionRecomputeScore ActionKind = "recompute_score"
	ActionEnqueueJob     ActionKind = "enqueue_job"
)

// The user-facing catalog's actions that have no lower-level kind: notify
// is delivery to a human, and draft_email creates a draft and never sends —
// the send is a separate, approval-gated act.
const (
	ActionNotify     ActionKind = "notify"
	ActionDraftEmail ActionKind = "draft_email"
)

// AllActionKinds is the closed set, in declaration order. The registry maps
// the user-facing catalog onto these; a kind with no executor fails the
// totality test rather than reaching a caller.
func AllActionKinds() []ActionKind {
	return []ActionKind{
		ActionCreateRecord, ActionUpdateRecord, ActionCreateTask, ActionAssignOwner,
		ActionAdvanceDeal, ActionSendEmail, ActionEmitFlowEvent, ActionRecomputeScore,
		ActionEnqueueJob, ActionNotify, ActionDraftEmail,
	}
}

type Action struct {
	Kind   ActionKind
	Target datasource.EntityRef
	Args   json.RawMessage

	// Deduplicated marks a create the effect-level claim folded: a sibling
	// instance's identical firing performed the write, so this action was
	// recorded but deliberately not executed (automation's applyCreate sets
	// it). Typed here rather than smuggled into Args so a trace reader can
	// render the fold instead of reporting a write that never happened.
	Deduplicated bool `json:"deduplicated,omitempty"`
}

// ApprovalToken references the typed, signed, single-use, effect-bound
// credential of ADR-0036; the approvals service owns its verification.
type ApprovalToken struct {
	Value string
}

// RunResult is audit-logged: a replayable trace of what was planned,
// approved, and applied.
type RunResult struct {
	RunID      ids.UUID
	Applied    []Action
	AuditLogID ids.UUID
}

// StagedApprovalError is the typed form of the "staged as approval"
// answer: a chat client shows the message, while a programmatic caller
// (the Surface-B runner) suspends on the id instead of parsing prose.
//
// AlreadyApproved says the id names a decision a human has ALREADY made and
// nobody has spent — the call was not staged, it was recognized. The two need
// different prose because they ask the caller for different things, and the one
// piece of advice that fits both ("wait for a human") is wrong for this half:
// waiting for a decision it already holds is what makes an agent stage the same
// question again.
type StagedApprovalError struct {
	ApprovalID      ids.ApprovalID
	AlreadyApproved bool
}

func (e *StagedApprovalError) Error() string {
	if e.AlreadyApproved {
		return fmt.Sprintf(
			"a human has already approved this exact call as approval %s — repeat it with \"approval_id\": %q and do not stage another: %s",
			e.ApprovalID, e.ApprovalID.String(), apperrors.ErrRequiresApproval)
	}
	return fmt.Sprintf(
		"staged as approval %s — once a human approves it, repeat this exact call with \"approval_id\": %q: %s",
		e.ApprovalID, e.ApprovalID.String(), apperrors.ErrRequiresApproval)
}

func (e *StagedApprovalError) Unwrap() error { return apperrors.ErrRequiresApproval }
