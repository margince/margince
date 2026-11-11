// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The closed automation catalog (B-E15.1, ADR-0035): the automation
// *types* a workspace may instantiate. Adding a member is a code
// change, never data — the anti-builder guard. The catalog, not the
// request, is the source of an instance's trigger/action/tier; params
// are validated here against each entry's schema, so a jsonb blob the
// engine would choke on never reaches the table.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// CatalogEntry describes one instantiable automation type. Key equals
// the backing workflow.Handler's Spec().Name — one vocabulary across
// the catalog, the engine, and the run records.
type CatalogEntry struct {
	Group        crmcontracts.AutomationCatalogEntryGroup
	Key          string
	Name         string
	Description  string
	Trigger      string // event type that fires it
	Action       string // closed workflow.ActionKind it plans
	Tier         string // auto_execute | confirmation_required (from the handler, never the caller)
	ParamsSchema map[string]any
	Validate     func(params map[string]any) error
	// ValidateRefs, when set, asks as the author whether the records the
	// params name exist for them: a list rule's lists, say.
	ValidateRefs func(ctx context.Context, lists Lists, params map[string]any) error
	// Seeded marks the entry as one of the SIX starter templates
	// SeedStarterAutomationsTx (automations.go) enrolls into a fresh
	// workspace on bootstrap (UAT.md:72 — "exactly the six
	// seeded templates"). An entry with Seeded false is still fully
	// instantiable through the API (the catalog stays the closed,
	// authorable set) — it just never lands in a workspace unasked.
	Seeded bool
}

// The JSON-schema vocabulary the closed catalog's hand-built schemas
// speak — one spelling of each keyword so the schema helpers read
// uniformly and a mistyped one is a build error, not a schema the
// editor form silently cannot render.
const (
	schemaTypeObject         = "object"
	schemaTypeString         = "string"
	schemaTypeInteger        = "integer"
	schemaTypeBoolean        = "boolean"
	schemaKeyType            = "type"
	schemaKeyProperties      = "properties"
	schemaKeyAdditionalProps = "additionalProperties"
	schemaKeyDescription     = "description"
	schemaKeyMinimum         = "minimum"
	schemaKeyMaximum         = "maximum"
	schemaKeyDefault         = "default"
	schemaKeyEnum            = "enum"
	schemaKeyFormat          = "format"
	schemaKeyRequired        = "required"
)

// minParamDays is the lower bound every "how many days" knob shares: a
// cadence of zero days is meaningless, so one is the floor for all of them.
const minParamDays = 1

// errNotAParameter is the 422 reason every closed-catalog validator
// returns for a params key outside its own schema. The routing rule keys
// (ruleKeyField/ruleKeyEquals/keyOwnerID) live beside their own schema in
// catalog_leadrouting_schema.go.
const errNotAParameter = "not a parameter of this automation type"

// reasonMustBeInteger is validateSingleIntParam's (and every hand-rolled
// counterpart's, e.g. automations_catalog_renewal.go's days_before check)
// shared refusal reason for a non-integer value.
const reasonMustBeInteger = "must be an integer"

// intParamProperty is the bounded-integer property shape every "how many
// days" knob shares — one spelling reused both by singleIntParamSchema's
// one-property object and by a larger schema (renewalReminderSchema)
// that embeds this SAME knob beside other properties.
func intParamProperty(defaultValue, maxValue int, description string) map[string]any {
	return map[string]any{
		schemaKeyType:        schemaTypeInteger,
		schemaKeyMinimum:     minParamDays,
		schemaKeyMaximum:     maxValue,
		schemaKeyDefault:     defaultValue,
		schemaKeyDescription: description,
	}
}

// singleIntParamSchema is the one-knob schema every "how many days"
// starter shares — due_in_days, no_activity_days, check_in_days, and
// days_before all take the identical bounded-integer shape and differ
// only in which key names the knob and what its own default/bounds are.
func singleIntParamSchema(key string, defaultValue, maxValue int, description string) map[string]any {
	return map[string]any{
		schemaKeyType:            schemaTypeObject,
		schemaKeyAdditionalProps: false,
		schemaKeyProperties: map[string]any{
			key: intParamProperty(defaultValue, maxValue, description),
		},
	}
}

// validateSingleIntParam is singleIntParamSchema's hand-rolled
// counterpart — the closed catalog owns its own tiny schemas, which
// does not buy a jsonschema dependency: an unknown knob or a mistyped
// or out-of-range value is a 422, never a stored blob the engine
// chokes on later.
func validateSingleIntParam(key string, maxValue int) func(params map[string]any) error {
	return func(params map[string]any) error {
		for k, value := range params {
			if k != key {
				return &ParamError{Field: "params." + k, Reason: errNotAParameter}
			}
			n, ok := value.(float64) // decoded JSON numbers arrive as float64
			if !ok || n != math.Trunc(n) {
				return &ParamError{Field: "params." + key, Reason: reasonMustBeInteger}
			}
			if n < float64(minParamDays) || n > float64(maxValue) {
				return &ParamError{
					Field:  "params." + key,
					Reason: fmt.Sprintf("must be between %d and %d", minParamDays, maxValue),
				}
			}
		}
		return nil
	}
}

// dueInDaysSchema is the shared shape of the create_task starters that
// key their one knob "due_in_days": how many days out the created task
// is due.
func dueInDaysSchema(defaultDays int, description string) map[string]any {
	return map[string]any{
		schemaKeyType:            schemaTypeObject,
		schemaKeyAdditionalProps: false,
		schemaKeyProperties: map[string]any{
			paramDueInDays:    intParamProperty(defaultDays, 30, description),
			paramTaskTimezone: taskTimezoneSchema(),
		},
	}
}

// validateDueInDays is dueInDaysSchema's validator.
func validateDueInDays(params map[string]any) error {
	if err := validateTaskTimezone(params); err != nil {
		return err
	}
	days := make(map[string]any, len(params))
	for key, value := range params {
		if key != paramTaskTimezone {
			days[key] = value
		}
	}
	return validateSingleIntParam(paramDueInDays, 30)(days)
}

// noActivityReminderSchema is no_activity_reminder's one-knob shape,
// keyed "no_activity_days" — the exact property name
// handlers_clock.go's noActivityDays reads off an instance's
// params, so the editor's schema and the handler's reader can never
// drift onto two different knob names for the same automation.
func noActivityReminderSchema() map[string]any {
	return quietReminderSchema("no_activity_days", defaultNoActivityDays, "pipeline_id", "pipeline")
}

// validateNoActivityReminderParams is noActivityReminderSchema's validator.
var validateNoActivityReminderParams = validateQuietReminderParams("no_activity_days", "pipeline_id")

// checkInCadenceSchema is check_in_cadence's own one-knob shape, keyed
// "check_in_days" — checkInCadenceDays' (handlers_clock.go)
// own reader, distinct from no_activity_reminder's key so a workspace
// may enable both with independent cadences.
func checkInCadenceSchema() map[string]any {
	return quietReminderSchema("check_in_days", defaultCheckInDays, "customer_list_id", "record_list")
}

// validateCheckInCadenceParams is checkInCadenceSchema's validator.
var validateCheckInCadenceParams = validateQuietReminderParams("check_in_days", "customer_list_id")

// noParamsSchema is the empty-knob schema for a starter with nothing to
// parameterize (stage_change_notify, post_meeting_recap): both fire
// unconditionally off their trigger alone, so the only valid params
// blob is the empty one.
func noParamsSchema() map[string]any {
	return map[string]any{
		schemaKeyType:            schemaTypeObject,
		schemaKeyAdditionalProps: false,
		schemaKeyProperties:      map[string]any{},
	}
}

// validateNoParams rejects any key at all — the starter reads no params.
func validateNoParams(params map[string]any) error {
	for key := range params {
		return &ParamError{Field: "params." + key, Reason: errNotAParameter}
	}
	return nil
}

// ParamError maps to 422 at the transport.
type ParamError struct {
	Field  string
	Reason string
}

func (e *ParamError) Error() string { return e.Field + ": " + e.Reason }

// FieldFault names the automation parameter that was rejected.
func (e *ParamError) FieldFault() (field, code, message string) {
	return e.Field, "invalid", e.Reason
}

// assignLeadOwnerName mirrors contacts.assignLeadOwnerName: the catalog
// key MUST equal the backing handler's Spec().Name, but a module never
// imports a sibling (ADR-0054 §9) — so this is its own copy of the
// literal, not a shared symbol. Kept in lockstep by
// internal/compose/leadrouting_config_test.go, which resolves this
// exact key against the real contacts.LeadRoutingWorkflow handler.
const assignLeadOwnerName = "assign_lead_owner"

// Catalog returns the closed automation library — the full authorable
// set: the six Seeded starter templates (seededCatalogEntries) plus the
// authorable-only entries (authorableOnlyCatalogEntries) and the list
// rules (listRuleEntries, listrules.go), none of which enroll into a fresh
// workspace unasked. Split into builders so each stays a short,
// single-purpose list rather than one long literal.
func Catalog() []CatalogEntry {
	entries := append(seededCatalogEntries(), authorableOnlyCatalogEntries()...)
	return append(entries, listRuleEntries()...)
}

// seededCatalogEntries is UAT.md:72's pinned six — the exact set
// SeedStarterAutomationsTx (automations.go) enrolls, ENABLED, into
// every fresh workspace. Each Key names the workflow.Handler
// automation's own StarterWorkflows registers under it.
func seededCatalogEntries() []CatalogEntry {
	return []CatalogEntry{
		{
			Key:          noActivityReminderName,
			Group:        crmcontracts.AutomationCatalogEntryGroupDeals,
			Name:         "Remind me when an active deal goes quiet",
			Description:  "Create a task for the owner of a quiet open deal in the selected pipeline. Genuine contact resets its clock.",
			Trigger:      noActivityScheduleMarker,
			Action:       string(ActionTypeCreateTask),
			Tier:         tierAutoExecute,
			ParamsSchema: noActivityReminderSchema(),
			Validate:     validateNoActivityReminderParams,
			Seeded:       true,
		},
		{
			Key:          renewalReminderName,
			Group:        crmcontracts.AutomationCatalogEntryGroupCustomers,
			Name:         "Renewal reminder",
			Description:  "Reminds an entity's owner as its configured renewal date approaches.",
			Trigger:      renewalScheduleMarker,
			Action:       string(ActionTypeCreateTask),
			Tier:         tierAutoExecute,
			ParamsSchema: renewalReminderSchema(),
			Validate:     validateRenewalReminderParams,
			Seeded:       true,
		},
		{
			Key:          stageChangeNotifyName,
			Group:        crmcontracts.AutomationCatalogEntryGroupDeals,
			Name:         "Tell the owner when someone moves their deal",
			Description:  "Notifies the deal's owner when someone else or an automation moves the deal, including closes. The owner's own moves stay in deal history.",
			Trigger:      eventDealStageChanged,
			Action:       string(ActionTypeNotify),
			Tier:         tierAutoExecute,
			ParamsSchema: stageChangeSchema(false),
			Validate:     validateStageChangeParams(false),
			Seeded:       true,
		},
		{
			Key:          routeLeadName,
			Group:        crmcontracts.AutomationCatalogEntryGroupInquiries,
			Name:         "Route new lead to a task",
			Description:  "Mints a follow-up task the moment a new lead is captured, so every lead gets a first next step.",
			Trigger:      eventLeadCreated,
			Action:       string(ActionTypeCreateTask),
			Tier:         tierAutoExecute,
			ParamsSchema: dueInDaysSchema(defaultRouteLeadDueInDays, "How many days out the follow-up task on the new lead is due."),
			Validate:     validateDueInDays,
			Seeded:       true,
		},
		{
			Key:          checkInCadenceName,
			Group:        crmcontracts.AutomationCatalogEntryGroupCustomers,
			Name:         "Keep in touch with selected customers",
			Description:  "Create a task for a customer list member's owner after the chosen period without genuine contact. An open equivalent reminder prevents another task.",
			Trigger:      checkInCadenceScheduleMarker,
			Action:       string(ActionTypeCreateTask),
			Tier:         tierAutoExecute,
			ParamsSchema: checkInCadenceSchema(),
			Validate:     validateCheckInCadenceParams,
			Seeded:       true,
		},
		{
			Key:          postMeetingRecapName,
			Group:        crmcontracts.AutomationCatalogEntryGroupMeetings,
			Name:         "Draft a follow-up after a meeting",
			Description:  "Drafts a follow-up once a customer meeting is held and has usable notes. The draft waits for a person and is never sent by the rule.",
			Trigger:      eventActivityCaptured,
			Action:       string(ActionTypeDraftEmail),
			Tier:         tierAutoExecute,
			ParamsSchema: noParamsSchema(),
			Validate:     validateNoParams,
			Seeded:       true,
		},
	}
}

// authorableOnlyCatalogEntries are real, instantiable catalog types that
// SeedStarterAutomationsTx never enrolls (Seeded false) — reachable
// through the API, never through the bootstrap floor.
func authorableOnlyCatalogEntries() []CatalogEntry {
	return []CatalogEntry{
		inquiryCatalogEntry(),
		{
			// AUTHORABLE, not seeded: the honest name for the owner-
			// assignment behaviour the OLD route_lead key carried before
			// AUTO-NOTE-2's reconciliation (migrations/core/0078 re-keys
			// any live row). Tier is auto_execute, not actionDefs' "dynamic" label
			// for assign_owner (catalog_actions.go) — the automation.tier
			// column's own CHECK constraint (migrations/core/0035) and the
			// wire contract's tier enum (crm.yaml) both close tier to
			// auto_execute|confirmation_required, and auto_execute is the honest default: no shipped
			// automation's own params carry a scale signal yet
			// (assign_owner_tier.go's own doc), so every real firing today
			// resolves single-entity — resolveAssignOwnerTier's own
			// zero-value case.
			Key:          assignLeadOwnerName,
			Group:        crmcontracts.AutomationCatalogEntryGroupInquiries,
			Name:         "Assign new leads an owner",
			Description:  "Assigns every new lead an owner: matching rules first, else round-robin across the pool, never exceeding capacity caps.",
			Trigger:      eventLeadCreated,
			Action:       string(ActionTypeAssignOwner),
			Tier:         tierAutoExecute,
			ParamsSchema: leadRoutingSchema(),
			Validate:     validateLeadRoutingParams,
			Seeded:       false,
		},
		{
			// AUTHORABLE, not seeded: a create_task-on-stage-change template
			// that predates the six-template pin (UAT.md:72). The six's own
			// stage-change template is stage_change_notify (above); this one
			// stays registered and instantiable rather than removed — an
			// extra available template, never a seeded seventh.
			Key:          stageChangeCreateTaskName,
			Group:        crmcontracts.AutomationCatalogEntryGroupDeals,
			Name:         "Add the next task when a deal reaches a stage",
			Description:  "Adds the chosen task when a deal enters a selected open stage. Each stage visit has its own task; repeated delivery creates no second task.",
			Trigger:      eventDealStageChanged,
			Action:       string(ActionTypeCreateTask),
			Tier:         tierAutoExecute,
			ParamsSchema: stageChangeSchema(true),
			Validate:     validateStageChangeParams(true),
			Seeded:       false,
		},
	}
}

// CatalogEntryByKey resolves one type; ok=false for anything outside
// the closed set.
func CatalogEntryByKey(key string) (CatalogEntry, bool) {
	for _, entry := range Catalog() {
		if entry.Key == key {
			return entry, true
		}
	}
	return CatalogEntry{}, false
}

// DueInDays reads the shared parameter with its per-type default —
// the one place the starters interpret their params blob.
func DueInDays(params json.RawMessage, defaultDays int) (int, error) {
	if len(params) == 0 {
		return defaultDays, nil
	}
	var decoded struct {
		DueInDays *int `json:"due_in_days"`
	}
	if err := json.Unmarshal(params, &decoded); err != nil {
		return 0, fmt.Errorf("crmagents: automation params: %w", err)
	}
	if decoded.DueInDays == nil {
		return defaultDays, nil
	}
	return *decoded.DueInDays, nil
}
