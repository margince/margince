// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// Every client-supplied reference a contract write carries — the deal or
// project it hangs off — is a row-scoped record, so create and patch gate them
// through the SAME rule here rather than each spelling its own.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// linkRef is one client-supplied reference to a row-scoped record: the table it
// names, and the id, when the request carried one.
type linkRef struct {
	table string
	id    *ids.UUID
}

func dealRef(id *ids.DealID) linkRef {
	if id == nil {
		return linkRef{table: dealTable}
	}
	return linkRef{table: dealTable, id: &id.UUID}
}

func projectRef(id *ids.ProjectID) linkRef {
	if id == nil {
		return linkRef{table: projectTable}
	}
	return linkRef{table: projectTable, id: &id.UUID}
}

// uuidRef names a link the patch body carried. An absent field is not a
// reference and is not checked; it leaves the column alone.
func uuidRef(table string, id *openapi_types.UUID) linkRef {
	if id == nil {
		return linkRef{table: table}
	}
	parsed := ids.UUID(*id)
	return linkRef{table: table, id: &parsed}
}

// ensureLinksVisible gates every client-supplied reference this write carries.
// Spelled once because create and patch must apply the same rule: the recurring
// defect in this tree is the second call site that forgets the first's gate.
func ensureLinksVisible(ctx context.Context, tx pgx.Tx, refs ...linkRef) error {
	for _, ref := range refs {
		if ref.id == nil {
			continue
		}
		if err := auth.EnsureLinkTarget(ctx, tx, ref.table, *ref.id); err != nil {
			return err
		}
	}
	return nil
}

// CrossCompanyLinkError reports a deal or project that belongs to a
// different company than the contract does.
type CrossCompanyLinkError struct{ Field string }

func (e *CrossCompanyLinkError) Error() string {
	return "the " + strings.TrimSuffix(e.Field, "_id") + " belongs to a different company than this contract"
}

// ensureLinksShareCompany refuses a contract whose deal or project belongs
// to another company.
//
// This is a VISIBILITY rule as much as a data-integrity one. The predicate that
// decides who may read a contract judges a deal-anchored contract by its DEAL
// alone, so pairing company A's contract with company B's deal would publish
// A's agreement to everyone who can see B — including through the events it
// emits. Two independent "can you see it" checks cannot catch that; only asking
// whether the two name the same company can.
func ensureLinksShareCompany(ctx context.Context, tx pgx.Tx, companyID ids.UUID, refs ...linkRef) error {
	for _, ref := range refs {
		if ref.id == nil {
			continue
		}
		// Nullable, because deal.company_id is — a deal may be worked
		// before anyone knows whose it is, and the create form leaves Company
		// optional. project.company_id is NOT NULL, so only the deal arm
		// ever reads absent.
		var linkedCompany *ids.UUID
		//nolint:gosec // the table name is a package literal from dealRef/projectRef, never client input
		query := "SELECT company_id FROM " + ref.table + " WHERE id = $1"
		err := tx.QueryRow(ctx, query, *ref.id).Scan(&linkedCompany)
		if errors.Is(err, pgx.ErrNoRows) {
			// EnsureLinkTarget already ran, so an absent row here means it was
			// archived or deleted in between; answer as it does.
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read %s company: %w", ref.table, err)
		}
		// A deal naming no company is not a company this contract disagrees
		// with. The leak this check exists against is A's agreement reaching
		// everyone who can see B's deal; with no B there is nobody it reaches
		// that the deal itself does not already admit.
		if linkedCompany == nil {
			continue
		}
		if *linkedCompany != companyID {
			return &CrossCompanyLinkError{Field: ref.table + "_id"}
		}
	}
	return nil
}
