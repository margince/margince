// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Record updates on a caller's transaction. Each entry point runs its pooled
// twin's body behind the same RBAC check, so it meets the rules a PATCH meets.

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// UpdateContactTx is UpdateContact on the caller's transaction.
//
// It files no share request when a colleague holds the contact. AskHolderToShare
// writes in a transaction of its own, so a caller wanting one calls it afterwards.
func (s *Store) UpdateContactTx(
	ctx context.Context, tx pgx.Tx, id ids.ContactID, in UpdateContactInput, cols CustomColumns,
) (crmcontracts.Contact, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return crmcontracts.Contact{}, err
	}
	return s.updateContactInTx(ctx, tx, id, in, cols.cols)
}

// UpdateCompanyTx is UpdateCompany on the caller's transaction.
func (s *Store) UpdateCompanyTx(
	ctx context.Context, tx pgx.Tx, id ids.CompanyID, in UpdateCompanyInput, cols CustomColumns,
) (crmcontracts.Company, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionUpdate); err != nil {
		return crmcontracts.Company{}, err
	}
	return s.updateCompanyInTx(ctx, tx, id, in, cols.cols)
}

// UpdateLeadTx is UpdateLead on the caller's transaction.
func (s *Store) UpdateLeadTx(
	ctx context.Context, tx pgx.Tx, id ids.LeadID, in UpdateLeadInput, cols CustomColumns,
) (crmcontracts.Lead, error) {
	if err := auth.Require(ctx, entityLead, principal.ActionUpdate); err != nil {
		return crmcontracts.Lead{}, err
	}
	return s.updateLeadInTx(ctx, tx, id, in, cols.cols)
}

// PrepareUpdateTx is Update split around a caller's transaction. The catalog
// read takes its own connection, so it runs now, and the returned write runs
// inside the caller's transaction.
//
// It serves the record types that carry an owner. Others answer
// UnsupportedEntityError, as Update does for a type it does not serve.
func (p *Provider) PrepareUpdateTx(ctx context.Context, t datasource.EntityType) (
	func(context.Context, pgx.Tx, datasource.UpdateInput) (datasource.EntityRef, error), error,
) {
	object := map[datasource.EntityType]string{
		datasource.EntityContact: entityContact, datasource.EntityCompany: entityCompany, datasource.EntityLead: entityLead,
	}[t]
	if object == "" {
		return nil, &datasource.UnsupportedEntityError{Type: string(t)}
	}
	cols, err := p.store.activeCustomColumns(ctx, object, principal.ActionUpdate)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, tx pgx.Tx, in datasource.UpdateInput) (datasource.EntityRef, error) {
		if in.Ref.Type != t {
			return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Ref.Type)}
		}
		raw, err := datasource.RawFields(in.Patch)
		if err != nil {
			return datasource.EntityRef{}, err
		}
		return p.updateTx(ctx, tx, in, raw, cols)
	}, nil
}

// updateTx routes one prepared write to its store entry point.
func (p *Provider) updateTx(
	ctx context.Context, tx pgx.Tx, in datasource.UpdateInput, raw json.RawMessage, cols CustomColumns,
) (datasource.EntityRef, error) {
	switch in.Ref.Type {
	case datasource.EntityContact:
		update, err := contactUpdateFrom(raw, in)
		if err != nil {
			return datasource.EntityRef{}, err
		}
		v, err := p.store.UpdateContactTx(ctx, tx, ids.From[ids.ContactKind](in.Ref.ID), update, cols)
		return ref(datasource.EntityContact, v.Id), err
	case datasource.EntityCompany:
		update, err := companyUpdateFrom(raw, in)
		if err != nil {
			return datasource.EntityRef{}, err
		}
		v, err := p.store.UpdateCompanyTx(ctx, tx, ids.From[ids.CompanyKind](in.Ref.ID), update, cols)
		return ref(datasource.EntityCompany, v.Id), err
	default:
		update, err := leadUpdateFrom(raw, in)
		if err != nil {
			return datasource.EntityRef{}, err
		}
		v, err := p.store.UpdateLeadTx(ctx, tx, ids.From[ids.LeadKind](in.Ref.ID), update, cols)
		return ref(datasource.EntityLead, v.Id), err
	}
}

// contactUpdateFrom decodes one contact patch, for Update and its Tx twin.
func contactUpdateFrom(raw json.RawMessage, in datasource.UpdateInput) (UpdateContactInput, error) {
	var req crmcontracts.UpdateContactRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return UpdateContactInput{}, err
	}
	update := contactUpdateInput(req, in.IfVersion)
	update.Trail = in.Trail
	update.Clear = in.Clear
	return update, nil
}

// companyUpdateFrom decodes one company patch, for Update and its Tx twin.
func companyUpdateFrom(raw json.RawMessage, in datasource.UpdateInput) (UpdateCompanyInput, error) {
	var req crmcontracts.UpdateCompanyRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return UpdateCompanyInput{}, err
	}
	update := companyUpdateInput(req, in.IfVersion)
	update.Trail = in.Trail
	update.Clear = in.Clear
	return update, nil
}

// leadUpdateFrom decodes one lead patch, for Update and its Tx twin.
func leadUpdateFrom(raw json.RawMessage, in datasource.UpdateInput) (UpdateLeadInput, error) {
	var req LeadUpdateRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return UpdateLeadInput{}, err
	}
	update := leadUpdateInput(req, in.IfVersion)
	update.Trail = in.Trail
	update.Clear = in.Clear
	return update, nil
}

// relationshipUpdateFrom decodes one relationship patch, as the others are
// decoded, so Update reads as routing alone.
func relationshipUpdateFrom(raw json.RawMessage, in datasource.UpdateInput) (UpdateRelationshipInput, error) {
	var req crmcontracts.UpdateRelationshipRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return UpdateRelationshipInput{}, err
	}
	return relationshipUpdateInput(req, in.IfVersion, in.Clear)
}
