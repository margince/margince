// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The options that put a MODEL behind a read surface: the cold-start read-back,
// the page scrape, and the meeting brief.
//
// Their own file beside serveroptions.go because they share the posture the
// first one states — without the option the operation is an explicit 501, since
// the api role must DECLARE its model path rather than pick one silently — and
// because each takes a brain rather than a store.

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/people"
)

// WithColdStart enables the cold-start read-back over the given fetch
// and model seams. Without it the operation stays an explicit 501 —
// the api role must DECLARE its model path, never pick one silently.
func WithColdStart(fetch PageFetcher, brain completer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.coldstartHandlers = coldstartHandlers{engine: &coldStartEngine{
			extract:   evidenceExtractor{fetch: fetch, brain: brain},
			approvals: approvals.NewService(InstallationDB(pool)),
		}}
	}
}

// WithScrape enables per-organization enrichment (scrapeCompany) over the same
// fetch and model seams as the read-back. Without it the operation stays an
// explicit 501 — the api role must DECLARE its model path, never pick one
// silently.
func WithScrape(fetch PageFetcher, brain completer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.scrapeHandlers = scrapeHandlers{engine: &scrapeEngine{
			extract:   evidenceExtractor{fetch: fetch, brain: brain},
			people:    people.NewStore(InstallationDB(pool)),
			approvals: approvals.NewService(InstallationDB(pool)),
		}}
	}
}

// WithBrief enables the Morning-Brief L2 ranker (B-E05.2) over the given
// model lane. Without it the brief still serves fully on the deterministic
// §10.1 composite — the L2 layer is advisory over that floor, never a
// prerequisite for the home surface.
func WithBrief(brain completer) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.WithL2Ranker(brain, s.log)
	}
}
