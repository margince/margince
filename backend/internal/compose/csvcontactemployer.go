// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (w *csvWriters) contactInputForRow(ctx context.Context, row migration.Row, typed map[string]any) (contacts.CreateContactInput, error) {
	in := contactCreateFrom(textFields(row.Fields), typed, w.sourceSystem, w.provenanceOf(row.ExternalID))
	if ref := strings.TrimSpace(textFields(row.Fields)[csvTargetCompanyRef]); ref != "" {
		answer, err := w.links.resolveCompany(ctx, ref)
		if err != nil {
			return in, err
		}
		in.Employer = contacts.EmployerChoice{Source: in.Source, Role: in.Title}
		if answer.Found {
			id := ids.From[ids.CompanyKind](answer.ID)
			in.Employer.CompanyID = &id
		}
	}
	return in, nil
}

func (w *csvWriters) recordCreatedEmployer(ctx context.Context, tx pgx.Tx, employer contacts.EmployerAttached) error {
	if !employer.CompanyCreated {
		return nil
	}
	if err := w.identities.RecordIdentityTx(ctx, tx, w.runID, w.sourceSystem, migration.ObjectCompany, employerImportKey(employer.CompanyID.String()), employer.CompanyID.UUID, migration.OriginCreated); err != nil {
		return err
	}
	clear(w.links.cache)
	clear(w.links.employers)
	return nil
}

func employerImportKey(id string) string { return "contact-employer:" + id }

func (w *csvWriters) resolveImportedEmployer(ctx context.Context, id ids.UUID, choice contacts.EmployerChoice) (migration.AssocResult, error) {
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		employer, err := w.contacts.ResolveContactEmployerTx(ctx, tx, ids.From[ids.ContactKind](id), choice)
		if err != nil {
			return err
		}
		return w.recordCreatedEmployer(ctx, tx, employer)
	})
	if why, refused := employerRefusalWhy(err); refused {
		return notLinkedFor(why), nil
	}
	return migration.AssocResult{Applied: err == nil}, err
}

func employerRefusalWhy(err error) (migration.Reason, bool) {
	if contacts.EmployerRefusalReason(err) == "" {
		return migration.Reason{}, false
	}
	return migration.Why(migration.ReasonEmployerRefused), true
}

func (w *csvWriters) previewImportedEmployer(ctx context.Context, row migration.Row, typed map[string]any) (contacts.ContactEmployerPreview, error) {
	in, err := w.contactInputForRow(ctx, row, typed)
	if err != nil {
		return contacts.ContactEmployerPreview{}, err
	}
	preview, err := w.contacts.PreviewContactEmployer(ctx, in)
	if err != nil {
		return preview, err
	}
	return w.links.previewPlannedEmployer(preview)
}
