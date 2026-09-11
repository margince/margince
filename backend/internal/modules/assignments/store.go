// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// assignmentVocabularyObject is the RBAC object the role catalog is gated by:
// the custom-field catalog's posture — everyone reads, admin/ops write — which
// is exactly this list's posture. deal_acquisition_source and lead_source are
// gated the same way, so the three administered vocabularies answer to one
// object rather than three near-identical entries in the policy matrix.
const assignmentVocabularyObject = "custom_field"

// Store owns record_role and record_assignment (data-seam ownership,
// ADR-0014 Am.1); every write rides the storekit audit+outbox shape in one
// transaction.
type Store struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
}

// NewStore builds the store.
func NewStore(db *database.DB) *Store { return &Store{db: db} }

// tx opens the transaction every read and write in this module runs inside,
// bound to the workspace the store holds.
func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return s.db.Tx(ctx, fn)
}

// unknownRecordType is the refusal both parent probes give a record type this
// system does not have. The route's own enum already rejects one, so reaching
// it means a caller bypassed the generated decoder.
func unknownRecordType() error {
	return &values.ParseError{
		Field:   recordTypeColumnOf,
		Code:    codeInvalidRecType,
		Message: "record_type is one of company, deal, project",
	}
}

// ensureParentReadable is the authority for READING a record's assignments: the
// caller must be able to see the record itself. Out of scope reads as not
// found, so asking for the assignments of a record you cannot open never
// confirms that the record exists.
func ensureParentReadable(ctx context.Context, tx pgx.Tx, rt crmcontracts.AssignmentRecordType, id ids.UUID) error {
	// BOTH halves, in this order. auth.Require answers "may this caller read
	// records of this kind at all", and EnsureVisible answers "is THIS row one
	// of theirs" — row scope alone admits a caller holding no read permission
	// on the parent kind, because a workspace-readable table answers yes for
	// every row.
	//
	// The three tables are spelled out rather than resolved through a variable:
	// the row-scope gate reads the table name from this call site, and a name it
	// cannot see is a reference it has to report as unbounded.
	switch rt {
	case crmcontracts.AssignmentRecordTypeCompany:
		if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
			return err
		}
		return auth.EnsureVisible(ctx, tx, "company", id)
	case crmcontracts.AssignmentRecordTypeDeal:
		if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
			return err
		}
		return auth.EnsureVisible(ctx, tx, "deal", id)
	case crmcontracts.AssignmentRecordTypeProject:
		if err := auth.Require(ctx, "project", principal.ActionRead); err != nil {
			return err
		}
		return auth.EnsureVisible(ctx, tx, "project", id)
	}
	return unknownRecordType()
}

// ensureParentWritable is the authority for CHANGING who is responsible: the
// caller must be able to write the record itself. This module adds no
// permission of its own — being allowed to edit a deal is exactly what being
// allowed to staff it means, and a separate assignment permission would drift
// from it the first time either changed.
//
// Live, not merely visible: staffing an archived record is a write nobody can
// act on, and the row lock EnsureWritableLive takes is what makes a concurrent
// archive and this write take turns.
func ensureParentWritable(ctx context.Context, tx pgx.Tx, rt crmcontracts.AssignmentRecordType, id ids.UUID) error {
	// THREE checks, narrowing each time: may this caller update records of this
	// kind at all, is this row one they can see, and is it one they may write.
	// auth.Require is the half row scope does not answer — without it a caller
	// holding no update permission on the parent kind still reaches this write,
	// because being the owner of a row is not the same as being allowed to
	// change rows of that type.
	//
	// HoldWritableLive is EnsureWritableLive followed by LockSubjectLive, and
	// the lock half is not a formality: the probe reads a SNAPSHOT, so without
	// it an archive committing between the check and the insert would leave an
	// assignment hanging on an archived record — the foreign key still
	// succeeds, because archiving does not delete the row.
	//
	// Holding the PARENT first is also what keeps this writer out of the
	// eraser's way: Art. 17 erasure locks its subject and then deletes the rows
	// hanging off it, so a writer taking its own rows first would deadlock
	// against it and cost somebody their erasure.
	switch rt {
	case crmcontracts.AssignmentRecordTypeCompany:
		if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
			return err
		}
		return auth.HoldWritableLive(ctx, tx, "company", id)
	case crmcontracts.AssignmentRecordTypeDeal:
		if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
			return err
		}
		return auth.HoldWritableLive(ctx, tx, "deal", id)
	case crmcontracts.AssignmentRecordTypeProject:
		if err := auth.Require(ctx, "project", principal.ActionUpdate); err != nil {
			return err
		}
		return auth.HoldWritableLive(ctx, tx, "project", id)
	}
	return unknownRecordType()
}

// parentOf collapses the three typed FK arms back to the single logical parent
// the API exposes. Exactly one is non-null — the record_assignment_one_parent
// CHECK guarantees it — so a row that reaches here with none is a schema
// violation rather than a case to handle politely.
func parentOf(company, deal, project *ids.UUID) (crmcontracts.AssignmentRecordType, ids.UUID, error) {
	switch {
	case company != nil:
		return crmcontracts.AssignmentRecordTypeCompany, *company, nil
	case deal != nil:
		return crmcontracts.AssignmentRecordTypeDeal, *deal, nil
	case project != nil:
		return crmcontracts.AssignmentRecordTypeProject, *project, nil
	}
	return "", ids.UUID{}, fmt.Errorf("record_assignment row has no parent")
}
