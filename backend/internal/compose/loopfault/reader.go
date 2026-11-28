// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package loopfault supplies a read-only failure boundary for worker certification.
package loopfault

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Failure may refuse a read; it cannot return records or authorize a write.
type Failure func(context.Context, datasource.EntityRef) error
