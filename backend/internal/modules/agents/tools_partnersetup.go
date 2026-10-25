// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// PartnerSetUpFields are the terms a partner program starts with. There is no
// margin tier: a human sets it on the company's Partner tab.
type PartnerSetUpFields struct {
	Role          string
	Stage         *string
	NextStep      *string
	NextStepDueAt *time.Time
}

// PartnerSetUp is the contacts store's partner set-up, as the agent tool needs it.
type PartnerSetUp interface {
	// Check answers what SetUp would refuse for the caller and this company now.
	Check(ctx context.Context, companyID ids.UUID) error
	// SetUp returns the written partner as JSON, already masked for the caller.
	SetUp(ctx context.Context, companyID ids.UUID, fields PartnerSetUpFields) (json.RawMessage, error)
}

var setUpPartnerCopy = toolCopy{
	Purpose: "Set up a partner program on a company that has none, with a role, a relationship " +
		"stage and a next step.",
	Limits: "Waits for a human to approve. The margin tier is not an argument: it decides " +
		"commission on every later deal, so a human sets it on the company's Partner tab. " +
		"Refused when the company already has a partner program, and needs the partner.update " +
		"and company.update grants.",
	Instead: "Use update_record to change a company's other fields.",
	Retain: "A staged answer's approval id is what the retry carries. The answer is the " +
		"partner program as it now stands.",
}

type setUpPartnerArgs struct {
	CompanyID         ids.UUID `json:"company_id"`
	PartnerRole       string   `json:"partner_role"`
	RelationshipStage *string  `json:"relationship_stage"`
	NextStep          *string  `json:"next_step"`
	NextStepDueAt     *string  `json:"next_step_due_at"`
}

type setUpPartner struct {
	p        datasource.SystemOfRecordProvider
	partners PartnerSetUp
	language baselanguage.Resolver
}

const setUpPartnerToolName = "set_up_partner"

func (t setUpPartner) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: setUpPartnerToolName, Title: "Set up a partner program", Version: toolVersionV1,
		Description: setUpPartnerCopy.render(),
		Instead:     setUpPartnerCopy.Instead,
		// Waits for a human even when a passport acts as its user. The web app
		// sets a program up directly. An assistant naming a company a partner is
		// a request for someone to look at.
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierConfirmationRequired, WaitsEvenForItsUser: true,
		Unlisted:  true,
		OpenAPIOp: "setUpPartner",
		InputSchema: schema(`{"type":"object","required":["company_id","partner_role"],"properties":{
			"company_id":{"type":"string","format":"uuid","description":"The company to make a partner"},
			"partner_role":{"type":"string","enum":["hosting","consulting","strategic"]},
			"relationship_stage":{"type":"string","enum":["research","identified","contacted","in_conversation","fit_confirmed","agreement_pending","active","active_referring","dormant","no_fit"]},
			"next_step":{"type":"string"},
			"next_step_due_at":{"type":"string","format":"date"},
			"approval_id":{"type":"string","format":"uuid","description":"Set on approved retry"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[crmcontracts.Partner](),
	}
}

// decodeSetUpPartner reads the arguments, refusing a margin tier before the
// closed schema can answer with a vaguer "unknown argument".
func decodeSetUpPartner(in json.RawMessage) (setUpPartnerArgs, error) {
	var named map[string]json.RawMessage
	if err := json.Unmarshal(in, &named); err == nil {
		if _, tier := named["margin_tier"]; tier {
			return setUpPartnerArgs{}, marginTierIsHumanOnly()
		}
	}
	var args setUpPartnerArgs
	if err := decodeArgs(in, &args); err != nil {
		return args, err
	}
	// The closed vocabularies are the contract's, and an unknown word is a
	// refusal here, not a database CHECK failure after the approval is spent.
	if !crmcontracts.SetUpPartnerRequestPartnerRole(args.PartnerRole).Valid() {
		return args, &BadArgsError{Field: "partner_role", Cause: errors.New("partner_role must be hosting, consulting or strategic")}
	}
	if stage := args.RelationshipStage; stage != nil && !crmcontracts.SetUpPartnerRequestRelationshipStage(*stage).Valid() {
		return args, &BadArgsError{Field: "relationship_stage", Cause: errors.New("relationship_stage is not a partner relationship stage")}
	}
	return args, nil
}

// marginTierIsHumanOnly is the refusal for a call that names a tier.
func marginTierIsHumanOnly() error {
	return &BadArgsError{
		Field: "margin_tier",
		Cause: errors.New("the margin tier decides commission on every later deal, so a human sets it on the company's Partner tab"),
	}
}

func (t setUpPartner) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	args, err := decodeSetUpPartner(in)
	if err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewSetUpPartnerCall(t.p, t.partners, t.language,
		SetUpPartnerCommand{CompanyID: args.CompanyID, Role: args.PartnerRole}))
}

func (t setUpPartner) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	args, err := decodeSetUpPartner(in)
	if err != nil {
		return nil, err
	}
	fields := PartnerSetUpFields{Role: args.PartnerRole, Stage: args.RelationshipStage, NextStep: args.NextStep}
	if args.NextStepDueAt != nil {
		due, err := time.Parse(time.DateOnly, *args.NextStepDueAt)
		if err != nil {
			return nil, &BadArgsError{Field: "next_step_due_at", Cause: errors.New("next_step_due_at must be a date like 2026-11-30")}
		}
		fields.NextStepDueAt = &due
	}
	written, err := t.partners.SetUp(ctx, args.CompanyID, fields)
	if err != nil {
		return nil, err
	}
	// The write is committed. A caller holding the update grants but not the
	// read grants gets the store's own masked result and no evidence record.
	rec, err := t.p.Read(ctx, datasource.EntityRef{Type: datasource.EntityPartner, ID: args.CompanyID})
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return written, nil
	}
	if err != nil {
		return nil, err
	}
	noteRecord(ctx, rec)
	return rec.Fields, nil
}
