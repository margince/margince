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
	"unicode/utf8"

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
	// RedrivableWithoutDuplicating says whether applying this handler's effect a
	// SECOND time repeats a side effect nobody asked for.
	//
	// Apply is documented idempotent on IdempotencyKey(ev), and nothing enforces
	// that: no Apply in the tree reads the key. The promise costs nothing while
	// every run happens once, and becomes load-bearing the moment anything
	// re-drives one — a retry then means "the row lands where it already was"
	// for one handler and "the customer is told a second time" for another.
	//
	// The answer belongs HERE rather than on the action kind, because the kind
	// does not decide what runs: leadRouting plans assign_owner and applies it
	// through RouteLead, while the engine's own handlers plan the same kind and
	// apply it through applyAssignOwner. One vocabulary, two writes.
	//
	// FALSE IS THE ZERO VALUE, so a handler that has not considered the question
	// answers no. A caller re-driving reads this; a false answer means the run
	// needs a human rather than a button.
	RedrivableWithoutDuplicating bool
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

	// RetryAttempt distinguishes a re-driven firing from the one it retries.
	// Zero for an ordinary firing, and the engine's own retry path sets it
	// (automation's RetryRun) — a handler never reads it and never sets it.
	//
	// It exists because the run claim is UNIQUE on (handler, idempotency_key):
	// re-dispatching under the original key finds the failed run's own row,
	// takes no claim, and returns having applied nothing. The marker rides the
	// EVENT rather than being spliced into the key at one call site, because
	// runKey is read by seven recorders inside one run and they must all agree
	// on which row this firing is writing.
	RetryAttempt int
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

// DeclinedError is a handler saying it looked and there was nothing to do.
//
// A skip, never a failure: nothing went wrong, and a redelivery would meet the
// same record and reach the same answer. The engine records it on the run with
// this reason, which is what puts the answer in front of a reader — an Apply
// that returns an empty result instead reads as a clean success and throws the
// reason away.
//
// The reason is read verbatim by anybody holding automation:read, so it says
// what the CONDITION was and never carries an error's own message: no SQLSTATE,
// no table or column name.
type DeclinedError struct {
	Reason string
}

func (e *DeclinedError) Error() string { return e.Reason }

// Declined builds the skip a handler returns to say it acted on nothing.
func Declined(reason string) error { return &DeclinedError{Reason: reason} }

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
	// Summary is what the staging already composed for the human's inbox card,
	// repeated to the caller that wrote the arguments.
	//
	// Without it a caller is told a decision is pending and has to go read the
	// proposal back to learn what it itself proposed — which an agent relaying
	// to a contact does not do: it says "a change will be applied once approved"
	// and never says which. The card and this sentence are the same sentence on
	// purpose, so the contact and the agent are waiting on one described thing.
	//
	// Nothing new is disclosed. It describes THIS call, built from arguments
	// this caller supplied, and the human sees the same text.
	Summary string
}

// MaxStagedSummary bounds the summary this answer repeats.
//
// A summary is server prose with CALLER VALUES inside it — a field list, a
// message body quoted whole — and it lands in a transcript whose later prompts
// the same run reads. The inbox card renders it in a scrollable panel and can
// afford the whole thing; a refusal cannot, or a caller chooses how much this
// server writes back at it.
//
// It is deliberately the same 300 as httperr.MaxFaultText, the near sibling:
// both bound one server-authored sentence carrying caller values, and a reader
// comparing two refusals should not find two different ceilings for the same
// obligation. Not shared as a constant because platform is downstream of
// shared/ports and an error type here cannot import it — the tool surface
// re-applies the figure anyway, through agents.echoSafe, which is where this
// text is also ESCAPED before a model reads it.
const MaxStagedSummary = 300

// boundedSummary keeps the repeated summary to MaxStagedSummary bytes, cut on a
// rune boundary so the answer is never invalid UTF-8.
func boundedSummary(s string) string {
	if len(s) <= MaxStagedSummary {
		return s
	}
	cut := MaxStagedSummary
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func (e *StagedApprovalError) Error() string {
	// The summary leads, because it is the part a caller relays to a contact.
	// An empty one is a producer that stages without describing what it staged,
	// which is a real state (the automation engine's effects) rather than a
	// gap to fill with a placeholder sentence.
	what := ""
	if e.Summary != "" {
		what = " to " + boundedSummary(e.Summary)
	}
	if e.AlreadyApproved {
		return fmt.Sprintf(
			"a human has already approved this exact call%s, as approval %s — repeat it with \"approval_id\": %q and do not stage another: %s",
			what, e.ApprovalID, e.ApprovalID.String(), apperrors.ErrRequiresApproval)
	}
	return fmt.Sprintf(
		"staged as approval %s%s — tell the user what it would do, and once a human approves it repeat this exact call with \"approval_id\": %q: %s",
		e.ApprovalID, what, e.ApprovalID.String(), apperrors.ErrRequiresApproval)
}

func (e *StagedApprovalError) Unwrap() error { return apperrors.ErrRequiresApproval }
