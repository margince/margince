// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The six auto-execute commands: logging an activity, drafting a reply,
// re-associating an activity, running a report, and answering one staged
// proposal or a whole bundle of them. All six are 🟢 today, so nothing stages
// them and neither question below is reached on today's tiers — the same
// standing the seven nested commands (commandnested.go) have.
//
// They are registered anyway, for that file's reason: a tier floor (#982)
// tightening one makes this the answer a human decides from, and the only
// alternative a route can offer is its own shape. That shape happens to name
// the right record for three of these and nothing at all for the fourth
// (runReport's route carries a `{report}` key, not an `{id}`) — the kind of
// accident this seam replaces with an answer the operation states itself.
//
// What does NOT happen here is the other half of the seam. Their TOOLS gain no
// StageInfo: a 🟢 tool has no staging path to move a guard off, so there is no
// second answer for the resolver to displace — and giving one a StageInfo
// would change what Registry.Stageable reports about the verb, which is what
// the contract's per-record-type tier floor consults before it may tighten it.
// That is a tiering decision, not a seam one, and it is not this task's to
// take.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// LogActivityCommand is one logged activity, whichever door asked for it. The
// body IS the activity's fields — the route names no record, because the
// activity does not exist yet.
type LogActivityCommand struct {
	Fields json.RawMessage
}

// NewLogActivityCall binds one logged activity to the resolver that answers
// for it. Like createResolver's, it holds no dependency: a create names no ROW,
// so there is nothing for Guards to read and nothing for Subject to describe
// beyond the command's own fields.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewLogActivityCall(cmd LogActivityCommand) GovernedCall {
	return bind[LogActivityCommand](logActivityResolver{}, cmd)
}

type logActivityResolver struct{}

// Subject names the record TYPE with no id and no pin — the shape every create
// stages (createResolver, command.go), because there is no row yet for either
// to describe.
func (logActivityResolver) Subject(_ context.Context, cmd LogActivityCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: string(datasource.EntityActivity),
		Summary:    describeGenericWrite("Log", string(datasource.EntityActivity), cmd.Fields),
	}, nil
}

// Guards stands down. It does not validate the fields against a shape the way
// createResolver.Guards does: log_activity's body IS crm.yaml's
// CreateActivityRequest, which the provider re-validates strictly at execution,
// and this verb never went through createShapes at either door — restating that
// vocabulary here would be a second, drifting answer to a question the store
// already answers.
func (logActivityResolver) Guards(_ context.Context, _ LogActivityCommand) error {
	return nil
}

// DraftEmailCommand is one drafted reply, whichever door asked for it — the
// routed activity is the thread being answered. It does not carry the intent:
// neither question below reads it.
type DraftEmailCommand struct {
	ActivityID ids.UUID
}

// NewDraftEmailCall binds one draft to the resolver that answers for it,
// reading the anchor through the record seam.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewDraftEmailCall(records datasource.SystemOfRecordProvider, cmd DraftEmailCommand) GovernedCall {
	return bind[DraftEmailCommand](&draftEmailResolver{
		anchor: anchoredRecord{records: records, entityType: datasource.EntityActivity},
	}, cmd)
}

type draftEmailResolver struct {
	anchor anchoredRecord
}

// Subject names the ANCHOR the draft answers, and pins its version: a draft is
// composed FROM that thread's content, so an approval given for drafting
// against the thread as it stands should not survive the thread changing.
func (r *draftEmailResolver) Subject(ctx context.Context, cmd DraftEmailCommand) (StageInfo, error) {
	rec, err := r.anchor.row(ctx, cmd.ActivityID)
	if err != nil {
		return StageInfo{}, err
	}
	return StageInfo{
		TargetType:    string(datasource.EntityActivity),
		TargetID:      cmd.ActivityID,
		TargetVersion: &rec.Version,
		Summary:       fmt.Sprintf("Draft a reply to activity %s", cmd.ActivityID),
	}, nil
}

// Guards refuses an anchor the caller cannot see or whose authority lives
// elsewhere — the same two refusals patchResolver.Guards makes for its own
// target.
func (r *draftEmailResolver) Guards(ctx context.Context, cmd DraftEmailCommand) error {
	return r.anchor.refuse(ctx, cmd.ActivityID)
}

// RelinkActivityCommand is one activity re-association, whichever door asked
// for it: the routed activity and the record it is being linked to. It does
// not carry replace_existing_of_type — whether the link moves or is added
// alongside is the executor's business, and neither question below reads it.
type RelinkActivityCommand struct {
	ActivityID ids.UUID
	EntityType string
	EntityID   ids.UUID
}

// NewRelinkActivityCall binds one re-association to the resolver that answers
// for it, wrapped in the destination-tier question every relink door shares.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewRelinkActivityCall(records datasource.SystemOfRecordProvider, cmd RelinkActivityCommand) GovernedCall {
	return destinationTieredCall{
		GovernedCall: bind[RelinkActivityCommand](&relinkActivityResolver{
			activity: anchoredRecord{records: records, entityType: datasource.EntityActivity},
		}, cmd),
		entityType: cmd.EntityType,
		pinnable:   true,
	}
}

// destinationTieredCall is a bound relink — of one activity, a thread, or a
// named set — plus the one question this family's tier turns on: WHICH KIND
// of record it files the activities under.
//
// Every destination but one is an ordinary association a member can undo by
// relinking again. Filing under a PROJECT is not: it classifies the activity as
// commercial correspondence, and that classification is write-once in the
// database and monotonic in the product — relinking away does not lift it, and
// removing it takes a named person giving a written reason through the
// controller's release path. An agent that could do that unattended could put a
// six-year retention floor across a mailbox with nothing to undo it, which is a
// denial of the subject's Art. 17 right that the controller cannot reverse.
// The batch doors are the same decision at scale, so they share this wrapper
// rather than each spelling the question again.
type destinationTieredCall struct {
	GovernedCall
	entityType string
	// pinnable says whether ONE version can condition this move. A single
	// relink's can; a thread's and a named set's cannot, because they move many
	// activities and a pin applied per row would refuse every one except
	// whichever happened to match.
	pinnable bool
}

// tierInput shows the gate the destination type AND the version the activity
// was read at.
//
// The destination alone decides the TIER — the risk is a property of the kind of
// record, not of the activity's state. But a dynamic tier that resolves to
// auto-execute and can name no version is refused by the gate (auth/admit.go),
// deliberately, because an unpinned write would run unattended. Answering no
// version therefore raised every REST relink to approval whatever its
// destination, while the MCP door auto-executed the same operation — two doors
// disagreeing about one call.
//
// The read is the staging path's own (relinkActivityResolver.Subject), reached
// through the bound call this wraps, so both doors are pinned by the same
// reading of the same row.
//
// A call that cannot be described answers NO version rather than an error, for
// the reason the MCP door's ResolverInput gives: the gate then raises, which is
// the safe direction and keeps the resolver's contract that it may only RAISE.
// A batch — a thread or a named set — answers none either, and cannot: one
// version cannot speak for many activities, so those doors keep costing a
// decision, which is the honest answer for a move nothing can fence.
//
//nolint:unparam // the shape is dynamicTierCall's, not this call's: the other implementation resolves a tier from records it must read, and narrowing here would put the seam's two sides on different shapes
func (c destinationTieredCall) tierInput(ctx context.Context, _ json.RawMessage) (mcp.TierResolverInput, error) {
	resolved := mcp.TierResolverInput{Args: relinkTierArgsFor(c.entityType)}
	if !c.pinnable {
		return resolved, nil
	}
	info, err := StageSubject(ctx, c.GovernedCall)
	if err != nil {
		return resolved, nil //nolint:nilerr // an unreadable record raises to a human, it does not fail the call
	}
	// Zero is not a version any write can be conditioned on, so it is left
	// unreported rather than pinned (dealmove.go says the same).
	if info.TargetVersion != nil && *info.TargetVersion > 0 {
		resolved.ObservedVersion = info.TargetVersion
	}
	return resolved, nil
}

// relinkTierArgs is the shape relinkActivityTier reads. It carries the
// destination type alone, because that is the whole of what decides the tier.
type relinkTierArgs struct {
	EntityType string `json:"entity_type"`
}

// relinkTierArgsFor encodes the destination type for the resolver.
//
// Marshalling one string field cannot fail, and the unreachable branch is
// spelled anyway rather than discarded: it returns nil, which the resolver
// cannot parse and therefore reads as the approval-requiring case. That is the
// same answer an error would have to produce, so the fallback is the safe one
// by construction rather than by a caller remembering to check.
func relinkTierArgsFor(entityType string) json.RawMessage {
	encoded, err := json.Marshal(relinkTierArgs{EntityType: entityType})
	if err != nil {
		return nil
	}
	return encoded
}

// relinkActivityTier raises a relink onto a PROJECT to confirm-first and leaves
// every other destination auto-executing.
//
// The resolver may only ever RAISE, and it fails TOWARD the approval gate: an
// absent, malformed or unreadable destination resolves 🟡 rather than 🟢, so a
// shape this does not recognise costs a human's attention instead of an
// irreversible retention floor nobody chose.
func relinkActivityTier(in mcp.TierResolverInput) mcp.RiskTier {
	var args relinkTierArgs
	if err := json.Unmarshal(in.Args, &args); err != nil {
		return mcp.TierConfirmationRequired
	}
	if args.EntityType == string(datasource.EntityProject) {
		return mcp.TierConfirmationRequired
	}
	if !relinkTargets[args.EntityType] {
		return mcp.TierConfirmationRequired
	}
	return mcp.TierAutoExecute
}

type relinkActivityResolver struct {
	activity anchoredRecord
}

// Subject names the ACTIVITY the approval binds to — the row that changes —
// and pins its version, with the destination carried into the summary: moving
// a captured email onto THIS deal and onto that one are different decisions
// wearing one shape.
func (r *relinkActivityResolver) Subject(ctx context.Context, cmd RelinkActivityCommand) (StageInfo, error) {
	rec, err := r.activity.row(ctx, cmd.ActivityID)
	if err != nil {
		return StageInfo{}, err
	}
	return StageInfo{
		TargetType:    string(datasource.EntityActivity),
		TargetID:      cmd.ActivityID,
		TargetVersion: &rec.Version,
		Summary: fmt.Sprintf("Re-associate activity %s to %s %s",
			cmd.ActivityID, cmd.EntityType, cmd.EntityID),
	}, nil
}

// Guards refuses a destination type that is not a link target — the same
// vocabulary the store refuses on, asked before a human is — and then the
// activity itself, the same two ways patchResolver.Guards refuses its own
// target.
//
// It does not read the DESTINATION record. The store refuses one the caller
// cannot see, at execution, and closing that here would mean a second read
// against a type this resolver is not built around; it is the same bound
// readStageableLinks closes for the verbs that NAME their records, and this
// one is filed rather than implied away: margince/margince#1021 is
// where a target's visibility question gets its home.
func (r *relinkActivityResolver) Guards(ctx context.Context, cmd RelinkActivityCommand) error {
	if err := requireLinkTarget(cmd.EntityType); err != nil {
		return err
	}
	return r.activity.refuse(ctx, cmd.ActivityID)
}

// requireLinkTarget admits the record type an activity may be re-associated to.
//
// One function for both doors — predicate and sentence together: the staging
// path asks it through Guards above and the execution path through
// relinkActivity.Handle, which an approved retry would re-enter without passing
// Guards. Two copies of the membership test is how the two doors come to
// disagree about what a link target is.
func requireLinkTarget(entityType string) error {
	if relinkTargets[entityType] {
		return nil
	}
	return &BadArgsError{Cause: fmt.Errorf("entity_type %q is not a link target", entityType)}
}

// RunReportCommand is one report run, whichever door asked for it. The report
// KEY is the whole of it: the plan arguments narrow what is counted, and the
// engine — not this seam — owns which of them a report accepts.
type RunReportCommand struct {
	Report string
}

// NewRunReportCall binds one report run to the resolver that answers for it.
// It holds no dependency: a report names no record at all.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewRunReportCall(cmd RunReportCommand) GovernedCall {
	return bind[RunReportCommand](runReportResolver{}, cmd)
}

type runReportResolver struct{}

// Subject names NO record, and that is the honest answer rather than a gap: a
// report is an aggregate over rows the caller's own scope already bounds, so
// there is no row an approval could bind to, pin, or be probed against. What
// it does supply is the KEY — the one thing that says which aggregate is being
// released — where the route walk it replaces could only offer an empty target
// with no name attached.
func (runReportResolver) Subject(_ context.Context, cmd RunReportCommand) (StageInfo, error) {
	return StageInfo{Summary: fmt.Sprintf("Run report %s", cmd.Report)}, nil
}

// Guards stands down: the report key's vocabulary is the engine's catalog,
// which this module is handed rather than owns (ReportRunner, tools_report.go),
// and a key outside it is refused by the engine at execution with the catalog
// in hand. Restating it here would be a second answer that drifts the moment an
// installation's catalog does.
func (runReportResolver) Guards(_ context.Context, _ RunReportCommand) error {
	return nil
}

// DecideApprovalCommand is one person's answer to one staged proposal,
// whichever door asked for it. Approve is the answer itself, and it belongs to
// the command rather than to the route because the two routes that carry it are
// one decision with two verdicts.
type DecideApprovalCommand struct {
	ApprovalID ids.UUID
	Approve    bool
}

// NewDecideApprovalCall binds one decision to the resolver that speaks it.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewDecideApprovalCall(cmd DecideApprovalCommand) GovernedCall {
	return bind[DecideApprovalCommand](decideApprovalResolver{}, cmd)
}

type decideApprovalResolver struct{}

// Subject names the decision and NOT a target record, which is the one thing
// worth saying about it: the row this call acts on is the approval, and an
// approval is not a record the staging path can row-scope or version-pin. A
// target named here would be an authority object pointing at an authority
// object — so the summary carries what a human would need to read and the
// target stays absent, the way runReportResolver's does for a report key.
func (decideApprovalResolver) Subject(_ context.Context, cmd DecideApprovalCommand) (StageInfo, error) {
	return StageInfo{Summary: fmt.Sprintf("%s staged action %s", verdictWord(cmd.Approve), cmd.ApprovalID)}, nil
}

// Guards stands down: what may be decided is the approvals engine's own
// question, answered against the deciding person's grants, the target's row
// scope and the caps the credential carries — none of which this module holds.
// A second answer here would be a weaker copy that drifts.
func (decideApprovalResolver) Guards(_ context.Context, _ DecideApprovalCommand) error {
	return nil
}

// DecideBundleCommand is the same answer given to every still-waiting member of
// one act.
type DecideBundleCommand struct {
	BundleID ids.UUID
	Approve  bool
}

// NewDecideBundleCall binds one bundle decision to its resolver.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewDecideBundleCall(cmd DecideBundleCommand) GovernedCall {
	return bind[DecideBundleCommand](decideBundleResolver{}, cmd)
}

type decideBundleResolver struct{}

// Subject names the act, for the reason the single decision's does not name a
// record: a bundle is a grouping and never a second authority object, so there
// is nothing here to pin either.
func (decideBundleResolver) Subject(_ context.Context, cmd DecideBundleCommand) (StageInfo, error) {
	return StageInfo{Summary: fmt.Sprintf("%s every waiting proposal of act %s",
		verdictWord(cmd.Approve), cmd.BundleID)}, nil
}

// Guards stands down for decideApprovalResolver's reason, per member.
func (decideBundleResolver) Guards(_ context.Context, _ DecideBundleCommand) error {
	return nil
}

// verdictWord is the one spelling of a verdict in a summary a human reads.
func verdictWord(approve bool) string {
	if approve {
		return "Approve"
	}
	return "Reject"
}

// AnnotateBriefCommand is the overnight pass's findings, staged.
//
// It carries the counts rather than the prose. What an approval of this would
// bind to is "the findings this pass wrote onto your own brief for today" — the
// run is the acting principal's own and is resolved server-side, so there is no
// id to pin and nothing a decider could redirect. The prose itself would only
// make a staging summary that quotes a model back at a human as though the
// quote were the decision.
type AnnotateBriefCommand struct {
	Items     int
	Narrative bool
}

// NewAnnotateBriefCall binds one annotation pass to the resolver that answers
// for it. It holds no dependency: the command names no row, because the run it
// writes to is the caller's own and the server resolves it.
//
//nolint:ireturn // the erased command-and-resolver pair is this constructor's whole product, as every New*Call beside it
func NewAnnotateBriefCall(cmd AnnotateBriefCommand) GovernedCall {
	return bind[AnnotateBriefCommand](annotateBriefResolver{}, cmd)
}

type annotateBriefResolver struct{}

// Subject names the brief as a type with no id, the same shape every create
// stages: the run is resolved from the acting principal and today's local day,
// so there is no identifier for a decider to see or to change.
func (annotateBriefResolver) Subject(_ context.Context, cmd AnnotateBriefCommand) (StageInfo, error) {
	return StageInfo{
		TargetType: "brief",
		Summary:    describeAnnotation(cmd),
	}, nil
}

// Guards stands down. Everything worth refusing about an annotation is refused
// by the store — whose run, which items, which citations — and restating any of
// it here would be a second answer to a question already answered where the row
// is.
func (annotateBriefResolver) Guards(_ context.Context, _ AnnotateBriefCommand) error {
	return nil
}

// describeAnnotation says what the pass wrote, in counts.
func describeAnnotation(cmd AnnotateBriefCommand) string {
	switch {
	case cmd.Items == 0 && !cmd.Narrative:
		return "Record that tonight's pass ran and found nothing to say"
	case cmd.Items == 0:
		return "Write a summary of the night onto your morning brief"
	case cmd.Narrative:
		return fmt.Sprintf("Write a summary of the night and %d finding(s) onto your morning brief", cmd.Items)
	default:
		return fmt.Sprintf("Write %d finding(s) onto your morning brief", cmd.Items)
	}
}
