// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/readerrendering"
	"github.com/margince/margince/backend/internal/modules/activities"
)

type readerRenderingHandlers = readerrendering.Handlers

// newReaderRenderingService reads the text through the activities store's
// content read, the one that refuses a row outside the caller's audience.
func newReaderRenderingService(pool *pgxpool.Pool) *readerrendering.Service {
	return readerrendering.NewService(pool, activities.NewStore(InstallationDB(pool)), time.Now)
}

// WithReaderRendering binds the reader_rendering lane. Unbound, the endpoint
// answers 409 ai_not_configured.
func WithReaderRendering(brain completer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.readerRenderingHandlers = readerrendering.NewHandlers(newReaderRenderingService(pool).WithLane(brain))
	}
}
