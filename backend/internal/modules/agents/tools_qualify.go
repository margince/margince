// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// qualify_lead (interfaces.md §2.2, DECISIONS A15): gap-only agentic
// qualification. The tool fills ONLY fields that are both currently
// empty and deterministically inferable from the lead's own data, then
// surfaces what still needs a human — it never overwrites a value and
// never invents one (a fill without evidence is a guess, and guesses
// are absent by construction).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// qualificationFields is the lead's qualification surface, in the fixed
// order gaps are reported (derived from the contract's lead shape). Source
// is not on it: the column is NOT NULL and administered as a vocabulary, so
// it is never a gap this tool could fill.
var qualificationFields = []string{"email", "full_name", "company_name", "title"}

type qualifyLead struct {
	p            datasource.SystemOfRecordProvider
	consumerMail ConsumerMail
}

func (t qualifyLead) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "qualify_lead", Title: "Qualify a lead", Version: toolVersionV1,
		Description:   qualifyLeadCopy.render(),
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "getLead + updateLead",
		InputSchema: schema(`{"type":"object","required":["record_id"],"properties":{
			"record_id":{"type":"string","format":"uuid","description":"The lead to qualify"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[QualifyLeadResult](),
	}
}

func (t qualifyLead) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		RecordID ids.UUID `json:"record_id"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	rec, err := t.p.Read(ctx, datasource.EntityRef{Type: datasource.EntityLead, ID: args.RecordID})
	if err != nil {
		return nil, err
	}
	noteRecord(ctx, rec)
	var lead struct {
		Email       *string `json:"email"`
		FullName    *string `json:"full_name"`
		CompanyName *string `json:"company_name"`
		Title       *string `json:"title"`
	}
	if err := json.Unmarshal(rec.Fields, &lead); err != nil {
		return nil, fmt.Errorf("crmagents: lead %s read back with unreadable fields: %w", args.RecordID, err)
	}

	patch := map[string]string{}
	filled := map[string]QualifiedField{}
	if isBlank(lead.CompanyName) && !isBlank(lead.Email) {
		company, ok, err := t.companyFromEmail(ctx, *lead.Email)
		if err != nil {
			return nil, err
		}
		if ok {
			patch["company_name"] = company
			lead.CompanyName = &company
			filled["company_name"] = QualifiedField{
				Value:    company,
				Evidence: []ContextEvidence{{Source: "lead.email", Snippet: *lead.Email}},
			}
		}
	}

	if len(patch) > 0 {
		raw, err := json.Marshal(patch)
		if err != nil {
			return nil, err
		}
		// Pin the update to the version the fill decision was read from:
		// if the lead changed underneath, the honest answer is skew, not a
		// blind write over whatever it became.
		if _, err := t.p.Update(ctx, datasource.UpdateInput{
			Ref:       datasource.EntityRef{Type: datasource.EntityLead, ID: args.RecordID},
			Patch:     raw,
			Source:    ToolSource,
			IfVersion: &rec.Version,
		}); err != nil {
			return nil, err
		}
	}

	gaps := []string{}
	for _, field := range qualificationFields {
		var value *string
		switch field {
		case "email":
			value = lead.Email
		case "full_name":
			value = lead.FullName
		case "company_name":
			value = lead.CompanyName
		case "title":
			value = lead.Title
		}
		if isBlank(value) {
			gaps = append(gaps, field)
		}
	}
	return json.Marshal(QualifyLeadResult{RecordID: args.RecordID, Filled: filled, Gaps: gaps})
}

func isBlank(s *string) bool { return s == nil || strings.TrimSpace(*s) == "" }

// companyFromEmail derives a company name from a corporate mail address.
//
// Both halves are asked of `platform/freemail`, and neither is this file's to
// answer. The web door asks the same package the same two questions, and the
// answers have to match: a list compiled in here would disagree with the
// operator's own administered overlay, and a name derived by cutting the domain
// at its first dot would call "eu.docusign.net" a company named "Eu". Either
// way the two doors write different companies from one address, in front of a
// user rather than in a log.
//
// A blank derivation is reported as a GAP rather than filled with a guess,
// which is this tool's whole contract: a fill without evidence is a guess, and
// guesses are absent by construction.
func (t qualifyLead) companyFromEmail(ctx context.Context, email string) (string, bool, error) {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return "", false, nil
	}
	// The syntactic gate, not just a lowercase: a lead's email is a string an
	// outsider chose, and `jane@%` is a legal RFC 5322 address whose domain is
	// a LIKE wildcard.
	domain, ok := freemail.Hostname(email[at+1:])
	if !ok {
		return "", false, nil
	}
	if t.consumerMail == nil {
		// Unwired is not "not a provider". A registry built without the seam
		// cannot answer the question, and answering it anyway would derive a
		// company from an address an operator may have marked consumer — so
		// this refuses on exactly the terms an unreadable list refuses on,
		// rather than nil-panicking at the first lead with an email.
		return "", false, errors.New("crmagents: qualify_lead has no consumer-mail list wired")
	}
	consumer, err := t.consumerMail.IsConsumer(ctx, domain)
	if err != nil {
		return "", false, err
	}
	if consumer {
		return "", false, nil
	}
	name := freemail.DisplayName(domain)
	if name == "" {
		return "", false, nil
	}
	return name, true, nil
}
