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
