// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package authz

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordGrantState keeps grant provenance alongside access and expiry for review.
type RecordGrantState struct {
	Access    string
	ExpiresAt *time.Time
	GrantedBy ids.UUID
	Reason    *string
}

// CarriedRecordGrant compares source authority with the grant that would survive a merge.
type CarriedRecordGrant struct {
	SourceID      ids.UUID
	SourceVersion int64
	SubjectType   string
	SubjectID     ids.UUID
	Source        RecordGrantState
	TargetID      *ids.UUID
	TargetVersion *int64
	Before        *RecordGrantState
	After         RecordGrantState
	ActiveAfter   bool
}

// The reasons a merge gives a former owner no new grant on the survivor.
const (
	OwnerCarryInactive       = "inactive"
	OwnerCarryAlreadyOwner   = "already_owner"
	OwnerCarryAlreadyGranted = "already_granted"
)

// OwnerAccessCarry is what keeping one former owner's access on a merge's
// survivor did: the access granted, or the reason none was.
type OwnerAccessCarry struct {
	Owner   ids.UUID
	Access  string
	Skipped string
}
