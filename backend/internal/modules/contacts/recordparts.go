// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// RecordParts writes the parts this module keeps under a record: a company's
// facts and profile fields, a project's companies and stakeholders. It makes
// the store calls the REST routes make, and answers as they answer.
type RecordParts struct {
	store *Store
}

// NewRecordParts builds the record-part writer over a workspace-bound handle.
func NewRecordParts(db *database.DB) RecordParts {
	return RecordParts{store: NewStore(db)}
}

// projectCompanyRole reads SetProjectCompany's role. An omitted role is the
// default; a present one must say something, so an empty one is refused.
func projectCompanyRole(role *string) (string, error) {
	if role == nil {
		return "", nil
	}
	return httperr.RequireNonBlank(relationshipRoleField, *role)
}

// CreateCompanyFact states a fact, as POST /companies/{id}/facts does.
func (p RecordParts) CreateCompanyFact(ctx context.Context, companyID ids.UUID, body json.RawMessage) (json.RawMessage, error) {
	var req crmcontracts.CreateCompanyFactRequest
	if err := datasource.StrictDecode(body, &req); err != nil {
		return nil, err
	}
	out, err := p.store.CreateCompanyFact(ctx, ids.From[ids.CompanyKind](companyID),
		FactCreateInput{Category: string(req.Category), Field: req.Field, Value: req.Value})
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// UpdateCompanyFact corrects a fact's value.
func (p RecordParts) UpdateCompanyFact(ctx context.Context, companyID ids.UUID, key string, value string, ifVersion *int64) (json.RawMessage, error) {
	out, err := p.store.UpdateCompanyFact(ctx, ids.From[ids.CompanyKind](companyID), key,
		FactWriteInput{Value: &value, IfVersion: ifVersion})
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// ConfirmCompanyFact confirms an extracted fact.
func (p RecordParts) ConfirmCompanyFact(ctx context.Context, companyID ids.UUID, key string, ifVersion *int64) (json.RawMessage, error) {
	out, err := p.store.ConfirmCompanyFact(ctx, ids.From[ids.CompanyKind](companyID), key, FactWriteInput{IfVersion: ifVersion})
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// DeleteCompanyFact removes a fact the company does not state.
func (p RecordParts) DeleteCompanyFact(ctx context.Context, companyID ids.UUID, key string, ifVersion *int64) (json.RawMessage, error) {
	if err := p.store.DeleteCompanyFact(ctx, ids.From[ids.CompanyKind](companyID), key, FactWriteInput{IfVersion: ifVersion}); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"removed": true})
}

// UpdateCompanyProfileField states or corrects a profile field.
func (p RecordParts) UpdateCompanyProfileField(ctx context.Context, companyID ids.UUID, field, value string, ifVersion *int64) (json.RawMessage, error) {
	out, err := p.store.UpdateCompanyProfileField(ctx, ids.From[ids.CompanyKind](companyID), field,
		ProfileFieldWriteInput{Value: &value, IfVersion: ifVersion})
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// ConfirmCompanyProfileField confirms a profile field.
func (p RecordParts) ConfirmCompanyProfileField(ctx context.Context, companyID ids.UUID, field string, ifVersion *int64) (json.RawMessage, error) {
	out, err := p.store.ConfirmCompanyProfileField(ctx, ids.From[ids.CompanyKind](companyID), field,
		ProfileFieldWriteInput{IfVersion: ifVersion})
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// SetProjectCompany puts a company on a project, or re-roles it.
func (p RecordParts) SetProjectCompany(ctx context.Context, projectID, companyID ids.UUID, role *string) (json.RawMessage, error) {
	named, err := projectCompanyRole(role)
	if err != nil {
		return nil, err
	}
	on, err := p.store.SetProjectCompany(ctx, SetProjectCompanyInput{
		ProjectID: ids.From[ids.ProjectKind](projectID), CompanyID: ids.From[ids.CompanyKind](companyID), Role: named,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(crmcontracts.ProjectCompanyListResponse{Data: wireProjectCompanies(on)})
}

// RemoveProjectCompany takes a company off a project.
func (p RecordParts) RemoveProjectCompany(ctx context.Context, projectID, companyID ids.UUID) (json.RawMessage, error) {
	if err := p.store.RemoveProjectCompany(ctx, ids.From[ids.ProjectKind](projectID), ids.From[ids.CompanyKind](companyID)); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"removed": true})
}

// SetProjectStakeholder attaches a contact to a project with a role.
func (p RecordParts) SetProjectStakeholder(ctx context.Context, projectID, contactID ids.UUID, role string) (json.RawMessage, error) {
	rel, err := p.store.SetProjectStakeholder(ctx, SetProjectStakeholderInput{
		ProjectID: ids.From[ids.ProjectKind](projectID), ContactID: ids.From[ids.ContactKind](contactID), Role: role,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireRelationship(rel))
}

// RemoveProjectStakeholder detaches a contact from a project.
func (p RecordParts) RemoveProjectStakeholder(ctx context.Context, projectID, contactID ids.UUID) (json.RawMessage, error) {
	if err := p.store.RemoveProjectStakeholder(ctx, ids.From[ids.ProjectKind](projectID), ids.From[ids.ContactKind](contactID)); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"removed": true})
}
