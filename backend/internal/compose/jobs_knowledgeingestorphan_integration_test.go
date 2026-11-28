// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A worker killed mid-ingest leaves its job `running` and nothing to end it. The
// next ingest of the same document must start at once, and two runs of one
// document must never both do the work. These tests drive the real worker, the
// real store and a real River insert.

import (
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// gatedEmbedder answers a deterministic vector per input and counts the inputs
// it was given. With a gate it holds its first call until released, which is a
// run that is alive and in the middle of its work.
type gatedEmbedder struct {
	inputs  atomic.Int64
	entered chan struct{}
	release chan struct{}
	// identity is the binding it answers under, fake/embed@8 when empty.
	identity string
}

func (e *gatedEmbedder) EmbedIdentity() (string, int) {
	if e.identity == "" {
		return "fake/embed@8", 8
	}
	return e.identity, 8
}

func (e *gatedEmbedder) Embed(_ context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	if e.inputs.Add(int64(len(req.Inputs))) == int64(len(req.Inputs)) && e.entered != nil {
		close(e.entered)
		<-e.release
	}
	vectors := make([][]float32, len(req.Inputs))
	for i, in := range req.Inputs {
		sum := sha256.Sum256([]byte(in))
		vec := make([]float32, 8)
		for d := range vec {
			vec[d] = float32(int(sum[d])%7+1) / 10
		}
		vectors[i] = vec
	}
	return model.Embeddings{Vectors: vectors, Dims: 8}, nil
}

type orphanIngest struct {
	e        *integration.Env
	worker   *knowledgeIngestWorker
	doc      ids.UUID
	passages int
	blob     blobstore.Store
	attempt  int
}

func newOrphanIngest(t *testing.T, embedder *gatedEmbedder) *orphanIngest {
	t.Helper()
	e := integration.Setup(t)
	uploader := corpusUploader(e)
	blob := blobstore.NewMemory()
	worker := newKnowledgeIngestWorker(e.Pool, blob, embedder, slog.Default())
	corpus, err := worker.store.CreateCorpus(uploader, knowledge.NewCorpus{Name: "How-to", TopicStatement: "How this product is operated."})
	if err != nil {
		t.Fatalf("create corpus: %v", err)
	}
	body := strings.Repeat(strings.Repeat("The corpus answers only from what a workspace filed in it. ", 12)+"\n\n", 3)
	upload := knowledge.NewDocument{CorpusID: ids.UUID(corpus.Id), Filename: "operating.md", ContentType: "text/markdown", Content: strings.NewReader(body)}
	doc, err := worker.store.UploadDocument(uploader, upload, func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	return &orphanIngest{e: e, worker: worker, doc: ids.UUID(doc.Id), passages: len(knowledge.ChunkText(body)), blob: blob}
}

// corpusUploader is an admin who may file and remove corpus documents.
func corpusUploader(e *integration.Env) context.Context {
	all := principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	grants := map[string]principal.ObjectGrant{"knowledge_corpus": all, "knowledge_document": all}
	return e.As(e.Rep1, nil, principal.Permissions{RoleKeys: []string{"admin"}, Objects: grants, RowScope: principal.RowScopeAll})
}

func (o *orphanIngest) work(t *testing.T) error {
	t.Helper()
	return o.worker.Work(boundBackground(t), &river.Job[KnowledgeIngestArgs]{
		JobRow: &rivertype.JobRow{CreatedAt: clocktest.Now(t), Attempt: max(o.attempt, 1)},
		Args:   KnowledgeIngestArgs{DocumentID: o.doc},
	})
}

// state is what readiness and the document list read.
func (o *orphanIngest) state(t *testing.T) (status string, chunks int) {
	t.Helper()
	if err := o.e.Pool.QueryRow(context.Background(),
		`SELECT d.ingest_status, (SELECT count(*) FROM knowledge_chunk c WHERE c.document_id = d.id AND c.archived_at IS NULL)
		   FROM knowledge_document d WHERE d.id = $1`, o.doc).Scan(&status, &chunks); err != nil {
		t.Fatalf("reading the document's ingest: %v", err)
	}
	return status, chunks
}

func newHandbookIngest(t *testing.T) *orphanIngest {
	t.Helper()
	e := integration.Setup(t)
	o := &orphanIngest{e: e, worker: newKnowledgeIngestWorker(e.Pool, blobstore.NewMemory(), &gatedEmbedder{}, slog.Default())}
	o.doc = o.boot(t, nil)[0]
	return o
}

func (o *orphanIngest) boot(t *testing.T, queue knowledge.QueueIngest) (queued []ids.UUID) {
	t.Helper()
	if queue == nil {
		queue = func(_ context.Context, _ pgx.Tx, id ids.UUID) error { queued = append(queued, id); return nil }
	}
	ctx := principal.SystemActing(context.Background(), "system:handbook-corpus")
	if _, err := o.worker.store.ReconcileHandbook(ctx, func(context.Context, pgx.Tx) error { return nil }, queue); err != nil {
		t.Fatalf("reconcile the handbook: %v", err)
	}
	return queued
}

func (o *orphanIngest) mustIngest(t *testing.T) {
	t.Helper()
	if err := o.work(t); err != nil {
		t.Fatalf("the ingest: %v", err)
	}
	if status, chunks := o.state(t); status != "done" || chunks == 0 {
		t.Fatalf("the document is %q with %d passages, want done with its passages", status, chunks)
	}
}

func (o *orphanIngest) setChecksum(t *testing.T, sum string) (was string) {
	t.Helper()
	if err := o.e.Pool.QueryRow(context.Background(),
		`UPDATE knowledge_document d SET checksum = $2 FROM knowledge_document p WHERE d.id = p.id AND d.id = $1 RETURNING p.checksum`, o.doc, sum).Scan(&was); err != nil {
		t.Fatalf("set the checksum: %v", err)
	}
	return was
}

func TestAHandbookPageLeftRunningByAKilledWorkerIsReadAgainAtTheNextBoot(t *testing.T) {
	o := newHandbookIngest(t)
	ctx := context.Background()
	inserter, err := jobs.NewInserter(o.e.Pool, slog.Default())
	if err != nil {
		t.Fatalf("NewInserter: %v", err)
	}
	args := KnowledgeIngestArgs{DocumentID: o.doc}
	if err := inserter.Enqueue(ctx, args, knowledgeIngestInsertOpts()); err != nil {
		t.Fatalf("the killed worker's job: %v", err)
	}
	if _, err := o.e.Pool.Exec(ctx, `
		UPDATE river_job SET state = 'running', attempt = 1,
		       attempted_at = now() - interval '2 minutes', attempted_by = ARRAY['a_worker_that_was_killed']
		WHERE kind = $1`, args.Kind()); err != nil {
		t.Fatalf("orphaning the job: %v", err)
	}
	if _, err := o.e.Pool.Exec(ctx, `UPDATE knowledge_document SET ingest_status = 'running' WHERE id = $1`, o.doc); err != nil {
		t.Fatalf("opening the dead attempt's page: %v", err)
	}
	o.boot(t, knowledgeIngestEnqueue(inserter))
	var total, waiting int
	if err := o.e.Pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE state = 'available') FROM river_job WHERE kind = $1`, args.Kind()).Scan(&total, &waiting); err != nil || total != 2 || waiting != 1 {
		t.Fatalf("%d ingest jobs, %d waiting after the next boot, want the dead one and one new one (err %v)", total, waiting, err)
	}
	o.mustIngest(t)
}

// Older bytes than the row names wait, never ingest; a failed page is queued by the next boot.
func TestAWorkerHoldingOlderHandbookBytesThanTheRowNamesNeverIngestsThem(t *testing.T) {
	for _, age := range []time.Duration{time.Minute, 2 * knowledgeIngestSnoozeWindow} {
		t.Run(age.String(), func(t *testing.T) {
			o := newHandbookIngest(t)
			inWindow := age < knowledgeIngestSnoozeWindow
			if !inWindow {
				o.attempt = knowledgeIngestMaxAttempts
			}
			o.worker.now = func() time.Time { return clocktest.Now(t).Add(age) }
			current := o.setChecksum(t, "sha256:a-newer-release")
			err := o.work(t)
			_, snoozed := errors.AsType[*rivertype.JobSnoozeError](err)
			if status, chunks := o.state(t); snoozed != inWindow || err == nil || status == "done" || chunks != 0 {
				t.Fatalf("older bytes: err=%v status=%q passages=%d", err, status, chunks)
			}
			o.setChecksum(t, current)
			if status, _ := o.state(t); status == "failed" && !slices.Contains(o.boot(t, nil), o.doc) {
				t.Fatal("the next boot did not queue the failed page again although its checksum is unchanged")
			}
			o.mustIngest(t)
		})
	}
}

func TestAJobForADocumentAlreadyReadEndsWithoutReadingItAgain(t *testing.T) {
	embedder := &gatedEmbedder{}
	o := newOrphanIngest(t, embedder)
	if err := o.work(t); err != nil {
		t.Fatalf("the first job: %v", err)
	}
	read := embedder.inputs.Load()
	if err := o.work(t); err != nil {
		t.Fatalf("the second job: %v", err)
	}
	if got := embedder.inputs.Load(); got != read {
		t.Fatalf("the second job embedded %d more passages of a document that was already done", got-read)
	}
	if status, chunks := o.state(t); status != "done" || chunks != o.passages {
		t.Fatalf("the document is %q with %d passages, want done with %d", status, chunks, o.passages)
	}
}

func TestAnIngestWhoseHoldingConnectionDiedDoesNotBlockTheNext(t *testing.T) {
	o := newOrphanIngest(t, &gatedEmbedder{})
	ctx := context.Background()
	release, held, err := holdIngest(ctx, o.e.Pool, o.doc, slog.Default())
	if err != nil || !held {
		t.Fatalf("taking the lock: held=%v err=%v", held, err)
	}
	// The holder's connection is ended below, so this closes it instead of
	// unlocking, and either way hands the pool back its connection.
	t.Cleanup(release)
	if err := o.work(t); err != nil {
		t.Fatalf("a run against a live holder: %v", err)
	}
	if status, chunks := o.state(t); status != "queued" || chunks != 0 {
		t.Fatalf("a run against a live holder left the document %q with %d passages, want it untouched", status, chunks)
	}

	// The holder's process dies: the server drops its connection and its lock.
	// The 5 s wait returns once the backend is gone. Without it the lock can
	// outlive the call, and the next run reads a holder that is still dying.
	var killed int
	if err := o.e.Pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE pg_terminate_backend(pid, 5000)) FROM pg_locks
		WHERE locktype = 'advisory' AND granted
		  AND ((classid::bigint << 32) | (objid::bigint & 4294967295)) = hashtextextended($1, 0)`, "knowledge_ingest:"+o.doc.String()).Scan(&killed); err != nil || killed != 1 {
		t.Fatalf("ending the holder's connection: ended=%d err=%v", killed, err)
	}

	if err := o.work(t); err != nil {
		t.Fatalf("a run after the holder died: %v", err)
	}
	if status, chunks := o.state(t); status != "done" || chunks != o.passages {
		t.Fatalf("after the holder died the document is %q with %d passages, want done with %d", status, chunks, o.passages)
	}
}

func TestTwoRunsOfOneDocumentEmbedItsPassagesOnce(t *testing.T) {
	embedder := &gatedEmbedder{entered: make(chan struct{}), release: make(chan struct{})}
	o := newOrphanIngest(t, embedder)

	first := make(chan error, 1)
	go func() { first <- o.work(t) }()
	select {
	case <-embedder.entered:
	case <-time.After(30 * time.Second):
		t.Fatal("the first run never reached its embedding")
	}

	if err := o.work(t); err != nil {
		t.Fatalf("the second run: %v", err)
	}
	if got := embedder.inputs.Load(); got != int64(o.passages) {
		t.Fatalf("%d passages sent to the embedder while one run was alive, want the first run's %d only", got, o.passages)
	}
	close(embedder.release)
	if err := <-first; err != nil {
		t.Fatalf("the first run: %v", err)
	}
	if status, chunks := o.state(t); status != "done" || chunks != o.passages {
		t.Fatalf("the document is %q with %d passages, want done with %d: a second run cut under the first", status, chunks, o.passages)
	}
	if got := embedder.inputs.Load(); got != int64(o.passages) {
		t.Fatalf("%d passages embedded in all, want each once (%d)", got, o.passages)
	}
	release, held, err := holdIngest(context.Background(), o.e.Pool, o.doc, slog.Default())
	if err != nil || !held {
		t.Fatalf("the finished run kept the lock: held=%v err=%v", held, err)
	}
	release()
}

// A run that cannot take a connection for its lock reports a fault, and leaves
// the document as it was for the retry.
func TestAnIngestThatCannotTakeItsLockFailsAndLeavesTheDocument(t *testing.T) {
	o := newOrphanIngest(t, &gatedEmbedder{})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, held, err := holdIngest(cancelled, o.e.Pool, o.doc, slog.Default()); err == nil || held {
		t.Fatalf("holdIngest on a cancelled context = held %v, err %v; want a refusal", held, err)
	}
	row := &rivertype.JobRow{CreatedAt: clocktest.Now(t), Attempt: 1}
	job := &river.Job[KnowledgeIngestArgs]{JobRow: row, Args: KnowledgeIngestArgs{DocumentID: o.doc}}
	if err := o.worker.Work(cancelled, job); err == nil {
		t.Fatal("a run that could not take its lock reported success")
	}
	if status, _ := o.state(t); status == "ready" {
		t.Fatalf("the document reads %q after a run that never started", status)
	}
}
