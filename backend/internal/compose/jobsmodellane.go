// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The job kinds whose work is a MODEL call, registered as one group.
//
// Its own file beside jobs.go, which wires the runner. These belong together
// because they share a posture rather than a subject: none is gated on the lane
// it reads, for the reason the function's own comment gives at length.

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// addModelLaneJobs registers the kinds whose work is a model call: the site
// deep read, the voice build, the embed reindex, and the two refreshes that
// extract rates from a page. The deferred-build retry sweep rides with them
// because it fans out to the voice build and to nothing else.
//
// Not one of them is gated on the lane it reads, and that shared posture is
// what makes them a group. Something outside the runner enqueues every model
// call here — an api call, a human's confirm, an admin's refresh click, the
// retry sweep's own fan-out — so a row can already be waiting when a role with
// no lane configured comes up, and registering anyway is what makes that row
// fail with an actionable message instead of sitting queued behind a job
// nothing works. The embed DRIFT sweep takes the opposite posture for the
// opposite reason (embeddriftsweep.go): nothing but its own tick enqueues it,
// so there is no waiting row for a worker to answer.
func addModelLaneJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) {
	// The deep read is the one kind whose timeout the file cannot state,
	// because the crawl wall it is built from is an operator's (deepReadTimeout).
	addDeclaredWorkerWithTimeout[SiteDeepReadArgs](reg,
		newSiteDeepReadWorker(pool, cfg.DeepReadBrain, cfg.DeepReadFactBrain, cfg.DeepReadTriageBrain, log, cfg.DeepReadCaps, cfg.Blobstore),
		deepReadTimeout(cfg.DeepReadCaps))
	addDeclaredWorker[TranscriptProposeArgs](reg, newTranscriptProposeWorker(pool, cfg.TranscriptProposeBrain, log))
	addDeclaredWorker[GeocodeOrganizationArgs](reg, newGeocodeWorker(pool, cfg.Geocoder))
	addDeclaredWorker[CheckOrganizationVatArgs](reg, newVatCheckWorker(pool, cfg.VatChecker, nil))
	addDeclaredWorker[DocumentExtractArgs](reg, newDocumentExtractWorker(pool, cfg.DocumentExtractBrain, cfg.SendBlob, log))
	addDeclaredWorker[VoiceBuildArgs](reg, newVoiceBuildWorker(pool, cfg.VoiceBrain, log))
	// The weekly retrospective moved here when it grew a lane. It is
	// database-only in its measuring half and stays registered whatever the
	// role holds — only the sentence is absent without a lane — but a group
	// documented as taking no config is the wrong place for something that
	// reads one.
	addWeeklyReviewJobs(reg, pool, log, cfg.WeeklyReviewBrain, cfg.WeeklyMail)
	addDeclaredWorker[VoiceBuildRetryArgs](reg, &voiceBuildRetryWorker{store: ai.NewVoiceStore(InstallationDB(pool)), log: log})
	// The reindex is a dispatcher plus a workspace worker, and neither is
	// ticked: the api enqueues the dispatcher once per confirmed reindex
	// (jobs_embedreindex.go).
	addEmbedReindexJobs(reg, pool, cfg.Embedder)
	addKnowledgeIngestJobs(reg, pool, cfg.Blobstore, cfg.Embedder, log)
	// The mailed-card import, registered unconditionally: a .vcf is parsed and
	// not inferred, so there is no model lane for it to be missing. Its trigger
	// starts unconditionally too, and the two must agree — River discards a job
	// whose kind no worker claims, so a trigger without this registration would
	// drop every mailed card silently.
	addDeclaredWorker[VCardIngestArgs](reg, newVCardIngestWorker(pool, cfg.Blobstore, log))
	// Both refreshes read a source the deployment configures. An unconfigured
	// one — a nil brain, an empty url, no pricing sources — leaves the worker
	// registered and its producer proposing nothing, which is the honest
	// answer to "refresh from sources" when there are none.
	addDeclaredWorker[FxRateRefreshArgs](reg, newFxRefreshWorker(pool, cfg.FxExtractBrain, cfg.FxSourceURL, cfg.FxBootstrapCurrencies, log))
	addDeclaredWorker[AiModelRateRefreshArgs](reg, newModelCostRefreshWorker(pool, cfg.RateExtractBrain, cfg.ModelPricingSources, cfg.BoundModelIDs, log))
}
