// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The litigation-hold seam.
//
// `legal_hold` sits on five tables owned by three modules — contact, company
// and lead in contacts, deal in deals, project in projects — and a module may
// not write a sibling's table. So the compliance surface asks a port, and this
// is the edge compose injects: one entity type in, one owner's own gated write
// out.
//
// The dispatch is the whole of the file on purpose. Everything a hold actually
// DOES — the authority check, the row lock, the refusal of a no-op, the audit
// row carrying the stated reason — is storekit.SetLegalHold, called by each
// owner. Putting any of it here would make compose a sixth writer of records
// it does not own.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LegalHoldSeam routes a hold to the module that owns the record's table.
type LegalHoldSeam struct {
	contacts *contacts.Store
	deals    *deals.Store
	projects *projects.Store
}

// NewLegalHoldSeam builds the seam over the installation's own pool.
func NewLegalHoldSeam(pool *pgxpool.Pool) LegalHoldSeam {
	db := InstallationDB(pool)
	return LegalHoldSeam{
		contacts: contacts.NewStore(db),
		deals:    deals.NewStore(db, DealsInstallation()),
		projects: projects.NewStore(db),
	}
}

// legalHoldSeamPort is LegalHoldSeam as privacy declares it. The assertion is
// here rather than at the assignment so a signature drift fails where the seam
// is defined, naming both sides.
var _ privacy.LegalHoldWriter = LegalHoldSeam{}

// SetLegalHold hands the decision to the table's owner.
//
// The default arm refuses rather than falling through. The generator types
// entityType as a closed enum, so an unknown one cannot arrive over HTTP — but
// a value that reaches here unrecognised means the enum and this switch have
// drifted, and quietly doing nothing would report a hold that was never placed.
//
// Held by: TestTheHoldSeamRefusesARecordTypeNoModuleOwns
// (internal/compose/legalholdseam_test.go)
func (s LegalHoldSeam) SetLegalHold(
	ctx context.Context, entityType string, id ids.UUID, held bool, reason string,
) error {
	switch agentRecordType(entityType) {
	case recordTypeContact:
		return s.contacts.SetLegalHold(ctx, contacts.HeldContact, id, held, reason)
	case recordTypeCompany:
		return s.contacts.SetLegalHold(ctx, contacts.HeldCompany, id, held, reason)
	case recordTypeLead:
		return s.contacts.SetLegalHold(ctx, contacts.HeldLead, id, held, reason)
	case recordTypeDeal:
		return s.deals.SetLegalHold(ctx, id, held, reason)
	case recordTypeProject:
		return s.projects.SetLegalHold(ctx, id, held, reason)
	default:
		return fmt.Errorf("legal hold on %q: %w", entityType, apperrors.ErrInvalidArgument)
	}
}
