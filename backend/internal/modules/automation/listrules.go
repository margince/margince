// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The rules that watch a Live List: when a record joins or leaves it, follow
// up with a task, tell the rule's owner, or add the record to a Shortlist. A
// rule names exactly one list, and only a list some active rule names fires
// anything. The trigger is the list's periodic check, so a record that joins
// and leaves between two checks is never seen.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// The catalog keys of the three list rules.
const (
	listTaskName      = "list_membership_task"
	listNotifyName    = "list_membership_notify"
	listShortlistName = "list_membership_shortlist"
)

// The params a list rule reads.
const (
	paramListID      = "list_id"
	paramDirection   = "direction"
	paramShortlistID = "shortlist_id"
	paramDueInDays   = "due_in_days"
)

// The directions a rule watches, and the observed actions each one fires on.
const (
	directionEntered = "entered"
	directionLeft    = "left"
	directionEither  = "either"
)

// The schema formats that tell the editor to draw a list picker.
const (
	formatLiveList  = "live_list"
	formatShortlist = "shortlist"
)

const defaultListTaskDueInDays = 2

// listRuleDescription is what every list rule says about when it fires.
const listRuleDescription = " It fires from the list's check every 15 minutes, once for each record seen" +
	" joining or leaving. A record that joins and leaves between two checks is not seen. When one" +
	" check moves more than 100 records, the rule fires for none of them and pauses itself."

// listRuleEntries are the catalog entries of the three list rules.
func listRuleEntries() []CatalogEntry {
	entry := func(key, name, description string, action ActionType, extra map[string]any) CatalogEntry {
		return CatalogEntry{
			Key: key, Name: name, Description: description + listRuleDescription,
			Trigger: eventListEvaluated, Action: string(action), Tier: tierAutoExecute,
			ParamsSchema: listRuleSchema(extra), Validate: validateListRuleParams(extra),
			ValidateRefs: validateListRuleRefs,
		}
	}
	return []CatalogEntry{
		entry(listTaskName, "Follow up when a record joins or leaves a Live List",
			"Mints a follow-up task on each record that joins or leaves the list, for the record's owner.",
			ActionTypeCreateTask, map[string]any{
				paramDueInDays: intParamProperty(defaultListTaskDueInDays, 30, "How many days out the follow-up task is due."),
			}),
		entry(listNotifyName, "Tell me when a record joins or leaves a Live List",
			"Sends the rule's owner a notice for each record that joins or leaves the list.",
			ActionTypeNotify, nil),
		entry(listShortlistName, "Add to a Shortlist when a record joins or leaves a Live List",
			"Adds each record that joins or leaves the list to a Shortlist of the same record type. A record already on it stays as it is.",
			ActionTypeAddToShortlist, map[string]any{
				paramShortlistID: map[string]any{
					schemaKeyType: schemaTypeString, schemaKeyFormat: formatShortlist,
					schemaKeyDescription: "The Shortlist each record is added to.",
				},
			}),
	}
}

// listRuleSchema is the watched list and direction every list rule takes,
// plus what its action needs.
func listRuleSchema(extra map[string]any) map[string]any {
	properties := map[string]any{
		paramListID: map[string]any{
			schemaKeyType: schemaTypeString, schemaKeyFormat: formatLiveList,
			schemaKeyDescription: "The Live List this rule watches.",
		},
		paramDirection: map[string]any{
			schemaKeyType: schemaTypeString, schemaKeyEnum: []string{directionEntered, directionLeft, directionEither},
			schemaKeyDefault: directionEntered, schemaKeyDescription: "Whether the rule fires when a record joins, leaves, or either.",
		},
	}
	maps.Copy(properties, extra)
	return map[string]any{
		schemaKeyType: schemaTypeObject, schemaKeyAdditionalProps: false,
		schemaKeyProperties: properties, schemaKeyRequired: requiredListParams(extra),
	}
}

func requiredListParams(extra map[string]any) []string {
	required := []string{paramListID}
	if _, ok := extra[paramShortlistID]; ok {
		required = append(required, paramShortlistID)
	}
	return required
}

// validateListRuleParams checks the shape of a list rule's params; whether
// the lists exist for the author is validateListRuleRefs' question.
func validateListRuleParams(extra map[string]any) func(map[string]any) error {
	return func(params map[string]any) error {
		for _, key := range requiredListParams(extra) {
			if _, ok := params[key]; !ok {
				return &ParamError{Field: "params." + key, Reason: "is required"}
			}
		}
		for key := range params {
			if err := validateListRuleParam(params, key, extra); err != nil {
				return err
			}
		}
		return nil
	}
}

func validateListRuleParam(params map[string]any, key string, extra map[string]any) error {
	field := "params." + key
	switch key {
	case paramListID:
		return validateListRef(params, key)
	case paramShortlistID:
		if extra[paramShortlistID] == nil {
			return &ParamError{Field: field, Reason: errNotAParameter}
		}
		return validateListRef(params, key)
	case paramDirection:
		if d, ok := params[key].(string); !ok || actionsFor(d) == nil {
			return &ParamError{Field: field, Reason: "must be entered, left or either"}
		}
		return nil
	case paramDueInDays:
		if extra[paramDueInDays] == nil {
			return &ParamError{Field: field, Reason: errNotAParameter}
		}
		return validateListTaskDays(params, key)
	default:
		return &ParamError{Field: field, Reason: errNotAParameter}
	}
}

func validateListRef(params map[string]any, key string) error {
	if text, ok := params[key].(string); !ok || !isUUID(text) {
		return &ParamError{Field: "params." + key, Reason: "must name a list"}
	}
	return nil
}

func validateListTaskDays(params map[string]any, key string) error {
	n, ok := params[key].(float64) // decoded JSON numbers arrive as float64
	if !ok || n != math.Trunc(n) || n < minParamDays || n > 30 {
		return &ParamError{Field: "params." + key, Reason: "must be a whole number of days between 1 and 30"}
	}
	return nil
}

func isUUID(text string) bool {
	_, err := ids.Parse(text)
	return err == nil
}

// actionsFor is the observed actions a direction fires on; nil for a
// direction outside the closed three.
func actionsFor(direction string) []string {
	switch direction {
	case directionEntered, "":
		return []string{directionEntered}
	case directionLeft:
		return []string{directionLeft}
	case directionEither:
		return []string{directionEntered, directionLeft}
	default:
		return nil
	}
}

// validateListRuleRefs asks, as the author, whether the watched list is a
// live Live List they can find and the Shortlist one of its record type they
// may change. The same questions are asked again as the owner at fire time.
func validateListRuleRefs(ctx context.Context, lists Lists, params map[string]any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	rule, err := decodeListRuleParams(raw)
	if err != nil {
		return err
	}
	watched, err := lists.Find(ctx, rule.ListID)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return &ParamError{Field: "params." + paramListID, Reason: "is not a list you can find"}
	case err != nil:
		return err
	case !watched.Live:
		return &ParamError{Field: "params." + paramListID, Reason: "must be a Live List"}
	case watched.Archived:
		return &ParamError{Field: "params." + paramListID, Reason: "is archived"}
	case watched.Invalid:
		return &ParamError{Field: "params." + paramListID, Reason: "has a filter that no longer works"}
	}
	if rule.ShortlistID == nil {
		return nil
	}
	return validateShortlistRef(ctx, lists, *rule.ShortlistID, watched.EntityType)
}

func validateShortlistRef(ctx context.Context, lists Lists, id ids.UUID, entityType string) error {
	field := "params." + paramShortlistID
	target, err := lists.Find(ctx, id)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return &ParamError{Field: field, Reason: "is not a list you can find"}
	case err != nil:
		return err
	case target.Live:
		return &ParamError{Field: field, Reason: "must be a Shortlist"}
	case target.Archived:
		return &ParamError{Field: field, Reason: "is archived"}
	case target.EntityType != entityType:
		return &ParamError{Field: field, Reason: "must hold the same record type as the watched list"}
	}
	err = lists.CheckShortlist(ctx, id, entityType)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return &ParamError{Field: field, Reason: "is not a Shortlist you may change"}
	}
	return err
}

// listRuleParams is a list rule's params, decoded.
type listRuleParams struct {
	ListID      ids.UUID  `json:"list_id"`
	Direction   string    `json:"direction"`
	DueInDays   *int      `json:"due_in_days"`
	ShortlistID *ids.UUID `json:"shortlist_id"`
}

func decodeListRuleParams(raw json.RawMessage) (listRuleParams, error) {
	var out listRuleParams
	if err := json.Unmarshal(raw, &out); err != nil {
		return listRuleParams{}, fmt.Errorf("automation: reading a list rule's params: %w", err)
	}
	return out, nil
}

// listRule is the handler behind every list rule. The engine hands it one
// event per record a check saw changing (listrulefire.go), never the check.
type listRule struct {
	name   string
	action ActionType
	ex     Executors
}

// listRuleWorkflows returns the handlers behind the three list rules.
func listRuleWorkflows(ex Executors) []workflow.Handler {
	return []workflow.Handler{
		listRule{name: listTaskName, action: ActionTypeCreateTask, ex: ex},
		listRule{name: listNotifyName, action: ActionTypeNotify, ex: ex},
		listRule{name: listShortlistName, action: ActionTypeAddToShortlist, ex: ex},
	}
}

func (r listRule) Spec() workflow.Spec {
	// Not redrivable: a retry re-reads the check's event, which names the list
	// and not the record the failed firing was about.
	return workflow.Spec{Name: r.name, Trigger: workflow.Trigger{EventType: eventListEvaluated}, Tier: mcp.TierAutoExecute}
}

// listFiring is the payload of one per-record firing.
type listFiring struct {
	ListID        ids.UUID `json:"list_id"`
	ListName      string   `json:"list_name"`
	Action        string   `json:"action"`
	MemberEventID ids.UUID `json:"member_event_id"`
}

func decodeFiring(ev workflow.Event) (listFiring, bool) {
	var f listFiring
	if len(ev.Payload) == 0 || json.Unmarshal(ev.Payload, &f) != nil || f.MemberEventID.IsZero() {
		return listFiring{}, false
	}
	return f, true
}

// Match refuses anything but a per-record firing: the check's own event
// names the list, and acting on it would act on the list.
func (r listRule) Match(_ context.Context, ev workflow.Event) (bool, error) {
	_, ok := decodeFiring(ev)
	return ok, nil
}

func (r listRule) Plan(ctx context.Context, ev workflow.Event) (workflow.Effect, error) {
	firing, ok := decodeFiring(ev)
	if !ok {
		return workflow.Effect{}, errors.New("automation: a list rule was planned without a record")
	}
	rule, err := decodeListRuleParams(ev.Params)
	if err != nil {
		return workflow.Effect{}, err
	}
	switch r.action {
	case ActionTypeCreateTask:
		days := defaultListTaskDueInDays
		if rule.DueInDays != nil {
			days = *rule.DueInDays
		}
		// The task is read by whoever may read the record, so it names no
		// list: a list's name and membership stay with the list's own sharing.
		return ownedTaskEffectNoKey(ctx, r.ex, ev, firing.taskSubject(),
			ev.OccurredAt.AddDate(0, 0, days))
	case ActionTypeNotify:
		return r.planNotice(ev, firing)
	case ActionTypeAddToShortlist:
		if rule.ShortlistID == nil {
			return workflow.Effect{}, errors.New("automation: a Shortlist rule names no Shortlist")
		}
		args, err := json.Marshal(addListMemberArgs{ListID: *rule.ShortlistID})
		if err != nil {
			return workflow.Effect{}, err
		}
		return workflow.Effect{Actions: []workflow.Action{{Kind: workflow.ActionAddListMember, Target: ev.Entity, Args: args}}}, nil
	default:
		return workflow.Effect{}, fmt.Errorf("automation: list rule %s carries action %s", r.name, r.action)
	}
}

func (r listRule) planNotice(ev workflow.Event, firing listFiring) (workflow.Effect, error) {
	args, err := json.Marshal(notifyArgs{
		Recipient: ev.OwnerID,
		DedupeKey: r.IdempotencyKey(ev),
		Subject:   firing.ListName,
		Body:      fmt.Sprintf("A record %s the list.", firing.did()),
		Origin:    noticeOrigin(ev),
	})
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("automation: encoding the notify action: %w", err)
	}
	return workflow.Effect{Actions: []workflow.Action{{Kind: workflow.ActionNotify, Target: ev.Entity, Args: args}}}, nil
}

// taskSubject is a follow-up task's subject, which names no list.
func (f listFiring) taskSubject() string {
	if f.Action == directionLeft {
		return "Left a Live List"
	}
	return "Joined a Live List"
}

// did says what the record did, in the past tense a sentence needs.
func (f listFiring) did() string {
	if f.Action == directionLeft {
		return "left"
	}
	return "joined"
}

func (r listRule) Apply(ctx context.Context, _ workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	applied, err := ApplyActions(ctx, r.ex, eff)
	return workflow.RunResult{Applied: applied}, err
}

// IdempotencyKey is the observed change itself, so a redelivered check fires
// each rule once per record it saw.
func (r listRule) IdempotencyKey(ev workflow.Event) string {
	if firing, ok := decodeFiring(ev); ok {
		return r.name + ":" + firing.MemberEventID.String()
	}
	return r.name + ":" + ev.ID.String()
}

// addListMemberArgs names the Shortlist an add_list_member action writes.
type addListMemberArgs struct {
	ListID ids.UUID `json:"list_id"`
}

// applyAddListMember puts the firing's record on the rule's Shortlist. The
// owner's own list authority and row scope admit the change; the write is the
// engine's, recorded on the owner's behalf. A record already there is left as
// it is and the action reports itself deduplicated.
func applyAddListMember(ctx context.Context, ex Executors, action workflow.Action) (workflow.Action, error) {
	if ex.Lists == nil {
		return action, errors.New("automation: no lists seam configured, so a record cannot be added to a Shortlist")
	}
	var args addListMemberArgs
	if err := json.Unmarshal(action.Args, &args); err != nil {
		return action, fmt.Errorf("automation: reading the Shortlist to add to: %w", err)
	}
	actor, ok := principal.Actor(ctx)
	workspace, inWorkspace := principal.WorkspaceID(ctx)
	if !ok || !inWorkspace || actor.OnBehalfOf.IsZero() {
		return action, errors.New("automation: a Shortlist is changed only on a rule owner's behalf")
	}
	admit, err := ownerContext(ctx, ex.Authority, workspace, actor.OnBehalfOf)
	if err != nil {
		return action, err
	}
	added, err := ex.Lists.AddMember(ctx, admit, args.ListID, action.Target)
	if err != nil {
		return action, err
	}
	action.Deduplicated = !added
	return action, nil
}
