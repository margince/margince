// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storeQueues is the follow-up work one composition's contacts stores queue:
// the coordinate lookup an address write queues, and the register consultation
// a VAT number queues. WithGeocoding and WithVatChecking bind it.
//
// The stores that write a site read's accepted fields are built from a pool.
// They sit below the option that holds the job runner, so the composition
// carries the queues to them. A nil *storeQueues queues neither, as the
// worker's own stores do.
type storeQueues struct {
	mu       sync.RWMutex
	geocode  contacts.GeocodeEnqueue
	vatCheck contacts.VatCheckEnqueue
}

func (q *storeQueues) bindGeocode(enqueue contacts.GeocodeEnqueue) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.geocode = enqueue
}

func (q *storeQueues) bindVatCheck(enqueue contacts.VatCheckEnqueue) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.vatCheck = enqueue
}

// geocodeEnqueue resolves the binding when an address lands, so a store built
// before the option ran still queues. Unbound, it queues nothing.
func (q *storeQueues) geocodeEnqueue(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) error {
	if q == nil {
		return nil
	}
	q.mu.RLock()
	enqueue := q.geocode
	q.mu.RUnlock()
	if enqueue == nil {
		return nil
	}
	return enqueue(ctx, tx, companyID)
}

// vatCheckEnqueue is the binding as a store is built. A store that holds nil
// refuses a requested check as unconfigured, so the value is taken, not wrapped.
func (q *storeQueues) vatCheckEnqueue() contacts.VatCheckEnqueue {
	if q == nil {
		return nil
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.vatCheck
}

// capturing is the deployment's capture config with this composition's queues,
// for a capture sink the server builds.
func (s *Server) capturing() CaptureConfig {
	cfg := s.captureConfig
	cfg.queues = s.queues
	return cfg
}
