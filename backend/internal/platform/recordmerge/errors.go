// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordmerge holds the steps every two-record merge shares: resolving
// and admitting both ends, retiring the source, and re-homing the rows that
// name a record by id or by kind and id. The contact, company, lead and deal
// merges call it; what each record kind moves beyond that stays with its
// module.
package recordmerge

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// MergeSelfError maps to 422: a record cannot merge into itself.
type MergeSelfError struct{}

func (e *MergeSelfError) Error() string { return "source and target of a merge must differ" }

// FieldFault refuses a merge whose source and target are the same record.
func (e *MergeSelfError) FieldFault() (field, code, message string) {
	return "target_id", "merge_self", e.Error()
}

// AlreadyMergedError maps to 409: the source was already merged away; the
// pointer names where it went.
type AlreadyMergedError struct {
	Kind   string
	IntoID ids.UUID
}

func (e *AlreadyMergedError) Error() string { return e.Kind + " is already merged" }

// MergedTargetError maps to 422: the chosen survivor is itself archived
// or merged away — nothing can merge INTO a dead record.
type MergedTargetError struct{ Kind string }

func (e *MergedTargetError) Error() string {
	return "merge target " + e.Kind + " is archived; the survivor must be live"
}

// FieldFault refuses merging INTO a record that was itself already merged away.
func (e *MergedTargetError) FieldFault() (field, code, message string) {
	return "target_id", "merged_target", e.Error()
}
