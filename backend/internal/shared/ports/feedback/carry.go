// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package feedback defines the authorized feedback view carried by record merges.
package feedback

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StoredDecision describes a ledger row being moved, not a verdict applied to a current field value.
type StoredDecision struct {
	ID              ids.UUID
	ClaimKind       string
	ClaimKey        string
	Verdict         string
	CorrectedValue  *string
	Note            *string
	ValueShown      *string
	ValueCapturedAt *time.Time
	CapturedBy      string
	UpdatedAt       time.Time
	Version         int64
}

// CarriedDecision pairs a moving feedback row with any verdict already held by the survivor.
type CarriedDecision struct {
	Source StoredDecision
	Target *StoredDecision
}
