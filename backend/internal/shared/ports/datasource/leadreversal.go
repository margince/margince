// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LeadReversalReaderV2 reads a promoted lead, which ordinary live reads exclude.
type LeadReversalReaderV2 interface {
	ReadLeadForReversal(context.Context, ids.UUID) (Record, error)
}
