// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package authz

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"time"
)

type RecordGrantState struct {
	Access    string
	ExpiresAt *time.Time
	GrantedBy ids.UUID
	Reason    *string
}

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
