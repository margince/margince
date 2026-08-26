// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The worker half of filing a document into a corpus: read the stored bytes,
// cut them into passages, and record them against the document.
//
// The api never does this in-request. It stores the file, writes the row and
// queues this job in the same transaction, so a document is `queued` from the
// moment it exists and its progress is a plain read of its own row.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/platform/jobs"
	"github.com/gradionhq/margince/backend/internal/platform/vectorkit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// knowledgeIngestActor is the agent every write of an ingest is attributed to.
// Its own name rather than the uploader's: the uploader chose the file, the
// ingest chose the passages, and provenance that borrows a neighbour's name
// cannot be re-derived later.
const knowledgeIngestActor = "agent:knowledge-ingest"

// knowledgeIngestMaxAttempts is how many times an ingest is tried before the
// document is marked failed.
//
// It lives here rather than in api/jobs.yaml because this kind's InsertOpts are
// the caller's: a number in the file would be one nothing applies. Three is the
// house on-demand figure, and it is enough — the failures worth retrying here
// are a database blip or object storage being briefly unreachable, and a fourth
// attempt at a file whose bytes are simply not text buys nothing.
const knowledgeIngestMaxAttempts = 3

// KnowledgeIngestArgs is one queued ingest.
type KnowledgeIngestArgs struct {
	Workspace  ids.UUID `json:"workspace_id"`
	DocumentID ids.UUID `json:"document_id"`
}

// Kind is the string River persists for this job.
func (KnowledgeIngestArgs) Kind() string { return "knowledge_ingest" }

// WorkspaceID declares the one tenant this job's work belongs to, and is what
// binds the worker's context — a worker that picked its own could claim one
// workspace and work in another.
func (a KnowledgeIngestArgs) WorkspaceID() ids.UUID { return a.Workspace }

// knowledgeIngestInsertOpts routes the ingest to the AI/capture queue and
// deduplicates by args over the ACTIVE states only.
//
// The state restriction is the load-bearing half. River's default uniqueness
// window includes completed, so a document whose first ingest finished could
// never be re-ingested — and the re-embed pass needs exactly that. Restricting
// to the active states keeps an in-flight ingest deduped while letting a
// finished one be asked again.
func knowledgeIngestInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue:       aiCaptureQueue,
		MaxAttempts: knowledgeIngestMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	}
}

// knowledgeIngestPrincipal stamps the principal every write of this ingest is
// attributed to, and correlates the writes to the document they are about.
func knowledgeIngestPrincipal(ctx context.Context, documentID ids.UUID) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   knowledgeIngestActor,
	})
	return principal.WithCorrelationID(ctx, documentID)
}

// knowledgeIngestWorker turns one stored document into passages.
type knowledgeIngestWorker struct {
	store *knowledge.Store
	// embedder is the retrieval embed lane. Nil is a role started without AI
	// routing: the passages are still written and the document still finishes,
	// and the ask answers retrieval_unavailable rather than pretending the
	// corpus does not cover the question.
	embedder vectorkit.Embedder
	log      *slog.Logger
}

func newKnowledgeIngestWorker(pool *pgxpool.Pool, blob blobstore.Store, embedder vectorkit.Embedder, log *slog.Logger) *knowledgeIngestWorker {
	return &knowledgeIngestWorker{
		store:    knowledge.NewStore(InstallationDB(pool)).WithBlobstore(blob),
		embedder: embedder,
		log:      log,
	}
}

func (w *knowledgeIngestWorker) Work(ctx context.Context, job *river.Job[KnowledgeIngestArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	wsCtx = knowledgeIngestPrincipal(wsCtx, job.Args.DocumentID)

	err = w.ingest(wsCtx, job.Args.DocumentID)
	if err == nil {
		return nil
	}
	// The document went away under us — archived, or its corpus was. Not a
	// fault and not worth a retry: the work has no subject any more.
	if errors.Is(err, apperrors.ErrNotFound) {
		w.log.InfoContext(wsCtx, "corpus document vanished mid-ingest",
			"document_id", job.Args.DocumentID)
		return river.JobCancel(err)
	}
	// Terminal only once River has no attempt left. A document mid-retry stays
	// `running`, which readiness reads as "not ready yet" rather than as
	// "permanently broken" — writing `failed` on the first attempt would make
	// a transient database blip look like a bad file to whoever uploaded it.
	if job.Attempt >= knowledgeIngestMaxAttempts {
		if ferr := w.store.FailIngest(wsCtx, job.Args.DocumentID, ingestDetail(err)); ferr != nil {
			return jobs.FaultContext(ctx, fmt.Errorf("recording the failed ingest: %w", ferr))
		}
		return river.JobCancel(err)
	}
	return jobs.FaultContext(ctx, err)
}

// ingest is one attempt, start to finish.
func (w *knowledgeIngestWorker) ingest(ctx context.Context, documentID ids.UUID) error {
	src, err := w.store.BeginIngest(ctx, documentID)
	if err != nil {
		return err
	}
	text, err := w.readDocument(ctx, src.StorageKey)
	if err != nil {
		return err
	}
	chunks := knowledge.ChunkText(text)
	if err := w.store.WriteChunks(ctx, documentID, src.CorpusID, chunks); err != nil {
		return err
	}
	// Embedded before the document is called done: `done` is what readiness
	// reads, and a document reported finished while its passages carry no
	// vector is a corpus that answers "not covered" for prose it holds.
	if w.embedder != nil {
		if _, err := w.store.EmbedDocument(ctx, documentID, w.embedder); err != nil {
			return err
		}
	}
	return w.store.FinishIngest(ctx, documentID, len(chunks))
}

// readDocument streams the stored file into memory. Whole, deliberately: the
// chunker is a pure function over the text and the upload ceiling is what keeps
// the size bounded — a streaming chunker would have to carry boundary state
// across reads to gain nothing at these sizes.
func (w *knowledgeIngestWorker) readDocument(ctx context.Context, key string) (string, error) {
	body, err := w.store.OpenDocument(ctx, key)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := body.Close(); cerr != nil {
			w.log.WarnContext(ctx, "closing the stored corpus document", "err", cerr)
		}
	}()
	raw, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("reading the stored corpus document: %w", err)
	}
	return string(raw), nil
}

// ingestDetail is what the uploader is shown. A typed refusal states its own
// actionable sentence; anything else is a fault they cannot act on, so they get
// a sentence that says so rather than an internal error.
func ingestDetail(err error) string {
	var full *knowledge.CorpusFullError
	if errors.As(err, &full) {
		return full.Error()
	}
	var unsupported *knowledge.UnsupportedTypeError
	if errors.As(err, &unsupported) {
		return unsupported.Error()
	}
	return "This document could not be read into passages. Nothing about the file is known to be wrong — try uploading it again."
}

// addKnowledgeIngestJobs registers the ingest. It registers even with no object
// storage bound, and with no embed lane: a document queued on an installation
// missing either then reaches a state a person can act on, rather than sitting
// queued forever behind a worker nobody composed.
func addKnowledgeIngestJobs(reg *jobRegistry, pool *pgxpool.Pool, blob blobstore.Store, embedder vectorkit.Embedder, log *slog.Logger) {
	addDeclaredWorker[KnowledgeIngestArgs](reg, newKnowledgeIngestWorker(pool, blob, embedder, log))
}

// WithKnowledgeIngest enables the corpus upload on the api role: the file is
// stored and its ingest queued through the insert-only runner, because the api
// never chunks or embeds in-request. Without it the upload answers its explicit
// refusal rather than accepting a document nothing will ever read.
func WithKnowledgeIngest(inserter *jobs.Runner) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.knowledgeHandlers = knowledgeWithIngestQueue(s.knowledgeHandlers, knowledgeIngestEnqueue(inserter))
	}
}

// knowledgeIngestEnqueue is the seam the knowledge module takes: the enqueue
// runs inside the transaction that writes the document row, so a job can never
// wake before the row exists nor survive a rollback that leaves it pointing at
// nothing.
func knowledgeIngestEnqueue(inserter *jobs.Runner) knowledge.QueueIngest {
	return func(ctx context.Context, tx pgx.Tx, documentID ids.UUID) error {
		return inserter.EnqueueTx(ctx, tx, KnowledgeIngestArgs{
			Workspace:  storekit.MustWorkspace(ctx),
			DocumentID: documentID,
		}, knowledgeIngestInsertOpts())
	}
}
