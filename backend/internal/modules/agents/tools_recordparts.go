// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// RecordParts writes what is kept under a record rather than on it. That is an
// offer's lines, a company's facts and profile fields, and a project's
// companies and stakeholders. Each method is the store call its REST route
// makes, and answers as that route answers.
type RecordParts interface {
	AddOfferLine(ctx context.Context, offerID ids.UUID, line json.RawMessage) (json.RawMessage, error)
	UpdateOfferLine(ctx context.Context, offerID, lineID ids.UUID, line json.RawMessage) (json.RawMessage, error)
	RemoveOfferLine(ctx context.Context, offerID, lineID ids.UUID) (json.RawMessage, error)
	CreateCompanyFact(ctx context.Context, companyID ids.UUID, fact json.RawMessage) (json.RawMessage, error)
	UpdateCompanyFact(ctx context.Context, companyID ids.UUID, key, value string, ifVersion *int64) (json.RawMessage, error)
	ConfirmCompanyFact(ctx context.Context, companyID ids.UUID, key string, ifVersion *int64) (json.RawMessage, error)
	DeleteCompanyFact(ctx context.Context, companyID ids.UUID, key string, ifVersion *int64) (json.RawMessage, error)
	UpdateCompanyProfileField(ctx context.Context, companyID ids.UUID, field, value string, ifVersion *int64) (json.RawMessage, error)
	ConfirmCompanyProfileField(ctx context.Context, companyID ids.UUID, field string, ifVersion *int64) (json.RawMessage, error)
	SetProjectCompany(ctx context.Context, projectID, companyID ids.UUID, role *string) (json.RawMessage, error)
	RemoveProjectCompany(ctx context.Context, projectID, companyID ids.UUID) (json.RawMessage, error)
	SetProjectStakeholder(ctx context.Context, projectID, contactID ids.UUID, role string) (json.RawMessage, error)
	RemoveProjectStakeholder(ctx context.Context, projectID, contactID ids.UUID) (json.RawMessage, error)
}

// RegisterRecordPartsTool joins change_record_parts to the surface.
func RegisterRecordPartsTool(r *Registry, parts RecordParts) {
	r.Register(changeRecordParts{parts: parts})
}

// changeRecordParts is one verb with an action argument, not a router. Every
// action writes one part under one record, at one tier.
type changeRecordParts struct{ parts RecordParts }

// recordPartArgs are the arguments the actions take between them.
type recordPartArgs struct {
	Action     string          `json:"action"`
	OfferID    ids.UUID        `json:"offer_id"`
	LineItemID ids.UUID        `json:"line_item_id"`
	Line       json.RawMessage `json:"line"`
	CompanyID  ids.UUID        `json:"company_id"`
	ProjectID  ids.UUID        `json:"project_id"`
	ContactID  ids.UUID        `json:"contact_id"`
	FactKey    string          `json:"fact_key"`
	Category   string          `json:"category"`
	Field      string          `json:"field"`
	Value      *string         `json:"value"`
	Role       *string         `json:"role"`
	IfVersion  *int64          `json:"if_version"`
}

// recordPartActions names what each action needs, in the order a refusal says it.
var recordPartActions = map[string][]string{
	"add_offer_line":                {"offer_id", "line"},
	"update_offer_line":             {"offer_id", "line_item_id", "line"},
	"remove_offer_line":             {"offer_id", "line_item_id"},
	"add_company_fact":              {"company_id", "category", "field", "value"},
	"update_company_fact":           {"company_id", "fact_key", "value"},
	"confirm_company_fact":          {"company_id", "fact_key"},
	"remove_company_fact":           {"company_id", "fact_key"},
	"update_company_profile_field":  {"company_id", "field", "value"},
	"confirm_company_profile_field": {"company_id", "field"},
	"set_project_company":           {"project_id", "company_id"},
	"remove_project_company":        {"project_id", "company_id"},
	"set_project_stakeholder":       {"project_id", "contact_id", "role"},
	"remove_project_stakeholder":    {"project_id", "contact_id"},
}

func (t changeRecordParts) Spec() mcp.ToolSpec {
	actions := make([]string, 0, len(recordPartActions))
	for action := range recordPartActions {
		actions = append(actions, action)
	}
	slices.Sort(actions)
	return mcp.ToolSpec{
		Name: "change_record_parts", Title: "Change the parts of a record", Version: toolVersionV1,
		Description:   changeRecordPartsCopy.render(),
		Instead:       changeRecordPartsCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "addOfferLineItem/updateOfferLineItem/removeOfferLineItem/createCompanyFact/updateCompanyFact/" +
			"confirmCompanyFact/deleteCompanyFact/updateCompanyProfileField/confirmCompanyProfileField/" +
			"setProjectCompany/removeProjectCompany/setProjectStakeholder/removeProjectStakeholder",
		InputSchema: schema(`{"type":"object","required":["action"],"properties":{
			"action":{"type":"string","enum":` + mustJSON(actions) + `},
			"offer_id":{"type":"string","format":"uuid"},
			"line_item_id":{"type":"string","format":"uuid"},
			"line":{"type":"object","description":"The line to add, or the fields of it to change. quantity is required to add one; product_id is taken only on add.","properties":{
				"product_id":{"type":"string","format":"uuid"},
				"description":{"type":"string"},
				"quantity":{"type":"number"},
				"unit":{"type":"string"},
				"unit_price_minor":{"type":"integer"},
				"discount_pct":{"type":"number"},
				"tax_rate":{"type":"number"},
				"position":{"type":"integer"},
				"billing_model":{"type":"string","enum":["one_time","recurring"]},
				"billing_interval_months":{"type":"string","enum":["1","3","6","12"]},
				"interval_count":{"type":"integer"}},"additionalProperties":false},
			"company_id":{"type":"string","format":"uuid"},
			"project_id":{"type":"string","format":"uuid"},
			"contact_id":{"type":"string","format":"uuid"},
			"fact_key":{"type":"string","description":"One fact of the company, spelled <field>:<value_key> from its read"},
			"category":{"type":"string"},
			"field":{"type":"string"},
			"value":{"type":"string"},
			"role":{"type":"string"},
			"if_version":{"type":"integer","description":"The version you read, so a change made since is refused rather than overwritten"}},
			"additionalProperties":false}`),
		OutputSchema: schema(`{"type":"object"}`),
	}
}

func (t changeRecordParts) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var a recordPartArgs
	if err := decodeArgs(in, &a); err != nil {
		return nil, err
	}
	needs, known := recordPartActions[a.Action]
	if !known {
		return nil, &BadArgsError{Field: "action", Cause: fmt.Errorf("%q is not an action", a.Action)}
	}
	for _, need := range needs {
		if !a.names(need) {
			return nil, &BadArgsError{Field: need, Cause: fmt.Errorf("%s needs %s", a.Action, need)}
		}
	}
	return t.run(ctx, a)
}

func (t changeRecordParts) run(ctx context.Context, a recordPartArgs) (json.RawMessage, error) {
	p := t.parts
	switch a.Action {
	case "add_offer_line":
		return p.AddOfferLine(ctx, a.OfferID, a.Line)
	case "update_offer_line":
		return p.UpdateOfferLine(ctx, a.OfferID, a.LineItemID, a.Line)
	case "remove_offer_line":
		return p.RemoveOfferLine(ctx, a.OfferID, a.LineItemID)
	case "add_company_fact":
		fact, err := json.Marshal(map[string]string{"category": a.Category, "field": a.Field, "value": *a.Value})
		if err != nil {
			return nil, err
		}
		return p.CreateCompanyFact(ctx, a.CompanyID, fact)
	case "update_company_fact":
		return p.UpdateCompanyFact(ctx, a.CompanyID, a.FactKey, *a.Value, a.IfVersion)
	case "confirm_company_fact":
		return p.ConfirmCompanyFact(ctx, a.CompanyID, a.FactKey, a.IfVersion)
	case "remove_company_fact":
		return p.DeleteCompanyFact(ctx, a.CompanyID, a.FactKey, a.IfVersion)
	case "update_company_profile_field":
		return p.UpdateCompanyProfileField(ctx, a.CompanyID, a.Field, *a.Value, a.IfVersion)
	case "confirm_company_profile_field":
		return p.ConfirmCompanyProfileField(ctx, a.CompanyID, a.Field, a.IfVersion)
	case "set_project_company":
		return p.SetProjectCompany(ctx, a.ProjectID, a.CompanyID, a.Role)
	case "remove_project_company":
		return p.RemoveProjectCompany(ctx, a.ProjectID, a.CompanyID)
	case "set_project_stakeholder":
		return p.SetProjectStakeholder(ctx, a.ProjectID, a.ContactID, *a.Role)
	default:
		return p.RemoveProjectStakeholder(ctx, a.ProjectID, a.ContactID)
	}
}

// names reports whether the call named the argument an action needs.
func (a recordPartArgs) names(arg string) bool {
	switch arg {
	case "offer_id":
		return !a.OfferID.IsZero()
	case "line_item_id":
		return !a.LineItemID.IsZero()
	case "line":
		return len(a.Line) > 0
	case "company_id":
		return !a.CompanyID.IsZero()
	case "project_id":
		return !a.ProjectID.IsZero()
	case "contact_id":
		return !a.ContactID.IsZero()
	case "fact_key":
		return a.FactKey != ""
	case "category":
		return a.Category != ""
	case "field":
		return a.Field != ""
	case "value":
		return a.Value != nil
	case "role":
		return a.Role != nil
	default:
		return false
	}
}
