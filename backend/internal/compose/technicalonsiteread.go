// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading a company's site asks what that company publicly runs, in the same
// breath.
//
// The two answer one question between them. A deep read says what the company
// TELLS you — the words on its pages — and the technical lookup says what it
// demonstrably RUNS, from records it does not write. A reader who opened an
// account to look at it wants both, and wanting them at different times was an
// artefact of the lookup shipping behind its own button rather than a property
// of the product.
//
// So this lane hangs off every read that reaches a company, automatic or
// human-pressed, on the same terms as the logo and newsroom lanes beside it:
// additive to a read that already succeeded, and never able to fail one.
//
// It ENQUEUES rather than reads. The other two side lanes fetch inline because
// each is one bounded request; a technical lookup is three services, one of
// them a certificate log whose own pacer may hold a caller for five seconds
// before the query is even built. Doing that inline would park a deep-read
// worker — the scarcest kind in the fleet, since a crawl already holds one for
// minutes — on a wait that has nothing to do with crawling. The lookup has its
// own single-threaded queue for exactly this reason, and joining that queue is
// how a read stays a read.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// askWhatTheCompanyRuns queues the technical lookup for the company this read
// was about.
//
// Errors are logged and dropped, never returned. The dossier this read is about
// to close is complete without the lookup, and a queue that would not take the
// row is not a reason to fail a crawl that already succeeded — the scheduled
// sweep comes back round for the same company either way.
//
// Two ways it correctly does nothing. A read that resolved no organization has
// no company to look up: the domain-triage lane runs before an account exists,
// and reading a site to decide whether to CREATE a company cannot enrich one.
// And a deployment whose worker role registered no enricher rejects the kind at
// insert, which is the honest answer rather than a row nothing will ever work.
func (w *siteDeepReadWorker) askWhatTheCompanyRuns(ctx context.Context, claim people.SiteReadClaim) {
	if claim.OrganizationID == nil {
		return
	}
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		w.log.WarnContext(ctx, "technical lookup not queued after site read: no workspace on the job context",
			"organization", claim.OrganizationID.String())
		return
	}
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		w.log.WarnContext(ctx, "technical lookup not queued after site read",
			"organization", claim.OrganizationID.String(), "err", err)
		return
	}
	// Deduplicated by args while queued or running (technicalInsertOpts), so a
	// read of a company the sweep just nominated — or that a rep pressed the
	// button on a moment ago — joins that lookup instead of asking the same
	// three services twice.
	if _, err := client.Insert(ctx, TechnicalEnrichOrganizationArgs{
		Workspace:      workspace,
		OrganizationID: *claim.OrganizationID,
	}, technicalInsertOpts()); err != nil {
		w.log.WarnContext(ctx, "technical lookup not queued after site read",
			"organization", claim.OrganizationID.String(), "err", err)
	}
}
