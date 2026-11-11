// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

// DependentRecordsError refuses a merge of a record that extension rows point
// at. A merge retires the source, and those rows would be left naming a record
// that no longer answers; archiving the source keeps them reachable.
type DependentRecordsError struct {
	Kind  string
	Count int64
}

func (e *DependentRecordsError) Error() string {
	return fmt.Sprintf("%s has %d extension records that depend on it", e.Kind, e.Count)
}

// Is makes the refusal a conflict (409).
func (e *DependentRecordsError) Is(target error) bool { return target == apperrors.ErrConflict }

// MessageFault names the count and the way forward for the caller.
func (e *DependentRecordsError) MessageFault() (code, message string) {
	return "has_dependent_records", fmt.Sprintf(
		"%d extension records depend on this %s, so it cannot be merged away; archive it instead", e.Count, e.Kind)
}

// WithExtensions wires the extension tables a merge checks for dependents.
// Every merge-capable store carries it; TestEveryProductionEraserCarriesTheExtensionRegistry
// holds that.
func (s *Store) WithExtensions(r extprivacy.Registry) *Store {
	s.extensions = r
	return s
}

// refuseMergeWithDependents is the dependents check the three merge paths share.
// It counts at merge time. The retired source stays as an archived row, so an
// extension row that links to it still resolves.
func (s *Store) refuseMergeWithDependents(ctx context.Context, tx pgx.Tx, kind extdecl.RecordKind, source ids.UUID) error {
	count, err := s.extensions.CountDependents(ctx, tx, kind, source)
	if err != nil {
		return err
	}
	if count > 0 {
		return &DependentRecordsError{Kind: string(kind), Count: count}
	}
	return nil
}
