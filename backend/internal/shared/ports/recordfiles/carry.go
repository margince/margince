// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordfiles defines authorized content references carried by record merges.
package recordfiles

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CarriedFile exposes content identity for merge review without granting file access.
type CarriedFile struct {
	ID          ids.UUID
	Filename    string
	Title       *string
	ContentType *string
	ByteSize    *int64
	Checksum    *string
	ArchivedAt  *time.Time
	UpdatedAt   time.Time
}
