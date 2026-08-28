// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The mail connectors' bounce port, bound to the comms outbound ledger.
//
// Capture cannot import comms — the report arrives in a mailbox, but the row
// it is about belongs to the send path — so the edge is injected here, like
// every other cross-module edge.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type commsBounceSink struct{ store *comms.Store }

func (b commsBounceSink) RecordBounce(ctx context.Context, report connector.BounceReport) error {
	// The marked/not-marked answer stays here: a report naming mail this
	// installation never sent is a normal capture input, not a connector
	// fault, so the connector only ever hears about real write failures.
	_, err := b.store.RecordBounce(ctx, report.MessageID, report.Kind, report.Reason)
	return err
}

// newBounceSink binds delivery reports to the comms store, exactly as the
// send path constructs it.
func newBounceSink(pool *pgxpool.Pool) connector.BounceSink {
	return commsBounceSink{store: comms.NewStore(InstallationDB(pool), time.Now, activities.NewStore(InstallationDB(pool)))}
}
