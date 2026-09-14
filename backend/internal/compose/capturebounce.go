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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type commsBounceSink struct{ store *comms.Store }

// consentBounceObserver binds comms' BounceObserver seam to the consent
// module's address stop. comms owns the send ledger and knows a message died;
// consent owns what may be written to whom. Neither imports the other, so the
// edge is injected here like every other cross-module edge.
type consentBounceObserver struct{ store *consent.Store }

func (o consentBounceObserver) HardBounceTx(
	ctx context.Context, tx pgx.Tx, fact comms.HardBounceFact,
) error {
	return consent.RecordHardBounceTx(ctx, tx, consent.HardBounceFact{
		Address:    fact.Address,
		DeliveryID: fact.DeliveryID,
	})
}

func (b commsBounceSink) RecordBounce(ctx context.Context, report connector.BounceReport) error {
	// The marked/not-marked answer stays here: a report naming mail this
	// installation never sent is a normal capture input, not a connector
	// fault, so the connector only ever hears about real write failures.
	_, err := b.store.RecordBounce(ctx, report)
	return err
}

// newBounceSink binds delivery reports to the comms store, exactly as the
// send path constructs it.
// newBounceSink binds delivery reports to the comms store, exactly as the
// send path constructs it — plus the observer that stops the dead address.
//
// The observer is wired HERE and not on every comms store: this is the one
// construction that receives delivery reports, so it is the only one whose
// RecordBounce ever fires. A read-only store built elsewhere carrying a
// suppression writer would be a capability nothing uses and everything has to
// reason about.
func newBounceSink(pool *pgxpool.Pool) commsBounceSink {
	return commsBounceSink{store: comms.NewStore(
		InstallationDB(pool), time.Now, activities.NewStore(InstallationDB(pool)),
		comms.WithBounceObserver(consentBounceObserver{store: consent.NewStore(InstallationDB(pool))}),
	)}
}
