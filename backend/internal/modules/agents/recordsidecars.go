// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// RecordSidecars are the rows a detail page shows beside a record's own fields,
// as the REST reads behind that page answer them. Only the kinds the record
// type has are filled.
type RecordSidecars struct {
	CompanyProfileFields []crmcontracts.CompanyProfileField
	CompanyFacts         []crmcontracts.CompanyFact
	ContactProfileFields []crmcontracts.ContactProfileField
}

// RecordSidecarsFor reads one record's sidecars through the readers its REST
// routes use, so their RBAC and row-scope gates apply unchanged.
type RecordSidecarsFor func(ctx context.Context, recordType datasource.EntityType, id ids.UUID) (RecordSidecars, error)

// WithRecordSidecars injects the sidecar read. A registry composed without one
// answers read_record with the record's own fields alone.
func WithRecordSidecars(read RecordSidecarsFor) RegistryOption {
	return func(r *Registry) { r.recordSidecars = read }
}

// readRecordResult is read_record's answer: the record, and what its page
// shows beside it.
type readRecordResult struct {
	recordWithOwner
	ProfileFields []recordProfileField `json:"profile_fields,omitempty"`
	Facts         []recordFact         `json:"facts,omitempty"`
}

// recordProfileField is one profile field as read_record reports it. Field is
// the key change_record_parts takes.
type recordProfileField struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Source string `json:"source"`
	// Confirmed says a human typed or confirmed the value.
	Confirmed bool `json:"confirmed"`
}

// recordFact is one company fact. FactKey is the `<field>:<value_key>` the
// fact actions of change_record_parts name it by.
type recordFact struct {
	FactKey   string `json:"fact_key"`
	Category  string `json:"category"`
	Field     string `json:"field"`
	Value     string `json:"value"`
	Source    string `json:"source"`
	Confirmed bool   `json:"confirmed"`
}

// withSidecars adds the record's sidecars to its read.
func withSidecars(ctx context.Context, read RecordSidecarsFor, rec recordWithOwner) (readRecordResult, error) {
	out := readRecordResult{recordWithOwner: rec}
	if read == nil {
		return out, nil
	}
	sidecars, err := read(ctx, datasource.EntityType(rec.RecordType), rec.ID)
	if err != nil {
		return readRecordResult{}, err
	}
	for _, pf := range sidecars.CompanyProfileFields {
		out.ProfileFields = append(out.ProfileFields, recordProfileField{
			Field: string(pf.Field), Value: pf.Value, Source: string(pf.Source),
			Confirmed: pf.VerifiedAt != nil || pf.Source == crmcontracts.CompanyProfileFieldSourceHuman,
		})
	}
	for _, fact := range sidecars.CompanyFacts {
		out.Facts = append(out.Facts, recordFact{
			FactKey: string(fact.Field) + ":" + fact.ValueKey, Category: string(fact.Category),
			Field: string(fact.Field), Value: fact.Value, Source: string(fact.Source),
			Confirmed: fact.VerifiedAt != nil || fact.Source == crmcontracts.CompanyFactSourceHuman,
		})
	}
	contactFields, err := contactOnlyProfileFields(rec.Fields, sidecars.ContactProfileFields)
	if err != nil {
		return readRecordResult{}, err
	}
	out.ProfileFields = append(out.ProfileFields, contactFields...)
	return out, nil
}

// contactOnlyProfileFields keeps the contact profile fields no field of the
// contact's own row carries, since those are in Fields already. A suppressed
// claim is one the page no longer shows.
func contactOnlyProfileFields(fields json.RawMessage, rows []crmcontracts.ContactProfileField) ([]recordProfileField, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	var own map[string]json.RawMessage
	if err := json.Unmarshal(fields, &own); err != nil {
		return nil, fmt.Errorf("agents: reading a contact's own field names: %w", err)
	}
	var out []recordProfileField
	for _, pf := range rows {
		if _, carried := own[string(pf.Field)]; carried {
			continue
		}
		if pf.Verdict != nil && *pf.Verdict == crmcontracts.ContactProfileFieldVerdictSuppressed {
			continue
		}
		out = append(out, recordProfileField{
			Field: string(pf.Field), Value: pf.Value, Source: pf.Source,
			Confirmed: pf.Verdict != nil || strings.HasPrefix(pf.CapturedBy, "human:"),
		})
	}
	return out, nil
}
