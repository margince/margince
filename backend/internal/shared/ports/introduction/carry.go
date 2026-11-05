// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package introduction describes introduction effects across module boundaries.
package introduction

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// CarriedRequest makes participant changes and cancellation effects reviewable before a merge.
type CarriedRequest struct {
	ID                          ids.UUID
	Version                     int
	ContactBefore, ContactAfter ids.UUID
	ThroughBefore, ThroughAfter *ids.UUID
	StatusBefore, StatusAfter   string
	CancellationReason          string
}
