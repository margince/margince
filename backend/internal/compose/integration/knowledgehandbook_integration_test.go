// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The handbook this release carries, reconciled into its corpus.
//
// What the suite is really guarding is a claim the product makes to a reader:
// that a citation into the handbook quotes prose THIS build ships. Every case
// below is one way that claim can quietly stop being true — a page filed twice,
// a reworded page still answering out of its old passages, a withdrawn page
// still citable, a boot that took someone else's corpus with it.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// handbookBootCtx is the scope the api boot binds before it reconciles: the
// installation's workspace, the release's own system actor, and a correlation
// id. A human context would be the wrong instrument — the boot has no request
// to take one from, and every row this writes is attributed to the release
// rather than to a person.
func handbookBootCtx(ws ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:handbook-corpus",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// noSerialize stands in for the advisory lock the composition layer supplies.
// The lock serializes REPLICAS; a suite runs one, and taking a real lock here
// would test Postgres rather than this code.
func noSerialize(context.Context, pgx.Tx) error { return nil }

// recordingQueue is the ingest seam, remembering what it was asked to read.
// Which documents get re-queued is half of what this reconciliation decides —
// a page whose bytes did not move must NOT be re-read, or every rollout spends
// an embedding call per page per replica to arrive at the passages already in
// the table.
type recordingQueue struct{ queued []ids.UUID }

func (q *recordingQueue) queue() knowledge.QueueIngest {
	return func(_ context.Context, _ pgx.Tx, documentID ids.UUID) error {
		q.queued = append(q.queued, documentID)
		return nil
	}
}

func TestTheFirstBootFilesEveryPageThisBuildCarries(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := handbookBootCtx(e.WS)

	pages, err := handbook.Pages()
	if err != nil {
		t.Fatalf("reading the embedded handbook: %v", err)
	}

	var q recordingQueue
	written, err := store.ReconcileHandbook(ctx, noSerialize, q.queue())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if written != len(pages) {
		t.Fatalf("wrote %d pages, want the %d this build carries", written, len(pages))
	}
	if len(q.queued) != len(pages) {
		t.Fatalf("queued %d ingests, want %d — a filed page nothing reads is a page the ask cannot cite",
			len(q.queued), len(pages))
	}

	filed := filedPages(ctx, t, e)
	if len(filed) != len(pages) {
		t.Fatalf("filed %d rows, want %d", len(filed), len(pages))
	}
	for _, page := range pages {
		if _, ok := filed[page.Filename]; !ok {
			t.Errorf("%s is embedded but was not filed", page.Filename)
		}
	}
}

func TestASecondBootOfTheSameBuildChangesNothing(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := handbookBootCtx(e.WS)

	var first recordingQueue
	if _, err := store.ReconcileHandbook(ctx, noSerialize, first.queue()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	before := filedPages(ctx, t, e)

	var second recordingQueue
	written, err := store.ReconcileHandbook(ctx, noSerialize, second.queue())
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if written != 0 {
		t.Errorf("the second boot wrote %d pages; an unchanged build must write none", written)
	}
	if len(second.queued) != 0 {
		t.Errorf("the second boot queued %d ingests; every rollout would re-embed the whole handbook", len(second.queued))
	}
	// The ids too, not just the count: a reconciliation that deleted and
	// re-filed every page would also report a stable NUMBER of rows while
	// breaking every citation and download link already handed to a reader.
	after := filedPages(ctx, t, e)
	for filename, id := range before {
		if after[filename] != id {
			t.Errorf("%s changed id across a boot, so every citation naming it now points at nothing", filename)
		}
	}
}

func TestARewordedPageKeepsItsRowAndIsReadAgain(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := handbookBootCtx(e.WS)

	var first recordingQueue
	if _, err := store.ReconcileHandbook(ctx, noSerialize, first.queue()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	before := filedPages(ctx, t, e)

	// Stand in for the previous release having shipped different prose. The
	// embedded pages are fixed at compile time, so the row is moved to a
	// checksum this build does not carry — which is exactly the state an
	// upgrade leaves behind.
	target := "README.md"
	e.WsExec(t, `UPDATE knowledge_document SET checksum = 'a-previous-release', chunk_count = 7,
	                          ingest_status = 'done'
	                    WHERE filename = $1 AND managed_source = $2`, target, knowledge.HandbookSource)

	var second recordingQueue
	written, err := store.ReconcileHandbook(ctx, noSerialize, second.queue())
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if written != 1 {
		t.Fatalf("wrote %d pages, want exactly the one whose bytes moved", written)
	}
	after := filedPages(ctx, t, e)
	if after[target] != before[target] {
		t.Fatal("the reworded page was re-filed under a new id, breaking every citation that named it")
	}
	if len(second.queued) != 1 || second.queued[0] != before[target] {
		t.Fatalf("queued %v, want exactly the reworded page %v — prose nobody re-read is prose the ask cannot cite",
			second.queued, before[target])
	}
	// Its old passages must be gone in the same commit. Left behind, the ask
	// would answer out of the PREVIOUS release's wording and cite this one's
	// page for it — the failure the whole design exists to prevent.
	var status string
	var chunks int
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT ingest_status, chunk_count FROM knowledge_document WHERE id = $1`,
			before[target]).Scan(&status, &chunks)
	}); err != nil {
		t.Fatalf("re-reading the page: %v", err)
	}
	if status != "queued" || chunks != 0 {
		t.Fatalf("status=%q chunks=%d, want queued/0 — the old wording is still answerable", status, chunks)
	}
}

func TestAWithdrawnPageStopsBeingCitable(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := handbookBootCtx(e.WS)

	var first recordingQueue
	if _, err := store.ReconcileHandbook(ctx, noSerialize, first.queue()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	// A page an earlier release shipped and this one does not. Planted rather
	// than removed, for the same reason as above: the embed is fixed at compile
	// time, and this is the row an upgrade leaves behind.
	corpusID := handbookCorpusID(ctx, t, e)
	e.WsExec(t, `INSERT INTO knowledge_document
	    (corpus_id, filename, content_type, byte_size, storage_key, checksum, managed_source, captured_by)
	  VALUES ($1, 'withdrawn.md', 'text/markdown', 12, 'handbook:withdrawn.md', 'old-release', $2, 'system:test')`,
		corpusID, knowledge.HandbookSource)

	var second recordingQueue
	if _, err := store.ReconcileHandbook(ctx, noSerialize, second.queue()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if _, still := filedPages(ctx, t, e)["withdrawn.md"]; still {
		t.Fatal("a page this release does not ship is still filed, so the ask can still cite prose that was withdrawn")
	}
}

// TestTheReconciliationLeavesAWorkspacesOwnCorpusAlone is the guarantee the
// RBAC waiver for ReconcileHandbook rests on.
//
// The boot runs under a system principal that no object grant refuses and no
// row scope narrows, so nothing in the authorization layer stands between it
// and a corpus somebody built by hand. What stands there instead is that every
// statement it runs is keyed on managed_source, and this is the test that says
// so — without it the waiver's rationale is a claim nothing holds.
func TestTheReconciliationLeavesAWorkspacesOwnCorpusAlone(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())

	// A corpus a person made, with a document in it.
	human := e.As(e.Rep1, nil, corpusAdminPerms)
	mine, err := store.CreateCorpus(human, howTo("Our own pricing notes"))
	if err != nil {
		t.Fatalf("create the workspace's own corpus: %v", err)
	}
	e.WsExec(t, `INSERT INTO knowledge_document
	    (corpus_id, filename, content_type, byte_size, storage_key, checksum, captured_by)
	  VALUES ($1, 'pricing.md', 'text/markdown', 40, 'ws/knowledge/pricing', 'mine', 'human:test')`,
		mine.Id)

	ctx := handbookBootCtx(e.WS)
	var q recordingQueue
	if _, err := store.ReconcileHandbook(ctx, noSerialize, q.queue()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var name string
	var docs int
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT name FROM knowledge_corpus WHERE id = $1 AND archived_at IS NULL`,
			mine.Id).Scan(&name); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM knowledge_document WHERE corpus_id = $1 AND archived_at IS NULL`,
			mine.Id).Scan(&docs)
	}); err != nil {
		t.Fatalf("re-reading the workspace's own corpus: %v", err)
	}
	if name != "Our own pricing notes" || docs != 1 {
		t.Fatalf("the workspace's own corpus came back as %q with %d documents; the boot reached a corpus it does not own", name, docs)
	}
}

// TestARestartDoesNotTakeBackTheDefault holds the one behaviour here a reader
// would guess wrong. An administrator who points the palette at their own
// corpus has made a decision; a restart is not a decision, and re-asserting the
// flag every boot would undo their choice at a moment nothing connects to it.
func TestARestartDoesNotTakeBackTheDefault(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := handbookBootCtx(e.WS)

	var first recordingQueue
	if _, err := store.ReconcileHandbook(ctx, noSerialize, first.queue()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	// The handbook takes the default on an installation that had none — the
	// positive arm, here so a change making the flag never set could not pass
	// this test by satisfying the negative one below.
	if !defaultAskIsHandbook(ctx, t, e) {
		t.Fatal("the handbook did not take the default on an installation that had none")
	}

	human := e.As(e.Rep1, nil, corpusAdminPerms)
	theirs := howTo("Our own pricing notes")
	theirs.DefaultAsk = true
	if _, err := store.CreateCorpus(human, theirs); err != nil {
		t.Fatalf("moving the default to the workspace's own corpus: %v", err)
	}

	var second recordingQueue
	if _, err := store.ReconcileHandbook(ctx, noSerialize, second.queue()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if defaultAskIsHandbook(ctx, t, e) {
		t.Fatal("a restart took the default back from the corpus an administrator chose")
	}
}

func defaultAskIsHandbook(ctx context.Context, t *testing.T, e *Env) bool {
	t.Helper()
	var isHandbook bool
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT coalesce(bool_or(managed_source = $1), false) FROM knowledge_corpus
			  WHERE default_ask AND archived_at IS NULL`, knowledge.HandbookSource).Scan(&isHandbook)
	}); err != nil {
		t.Fatalf("reading which corpus holds the default: %v", err)
	}
	return isHandbook
}

func handbookCorpusID(ctx context.Context, t *testing.T, e *Env) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id FROM knowledge_corpus WHERE managed_source = $1 AND archived_at IS NULL`,
			knowledge.HandbookSource).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the handbook corpus: %v", err)
	}
	return id
}

// filedPages is filename → row id for the handbook's own documents.
func filedPages(ctx context.Context, t *testing.T, e *Env) map[string]ids.UUID {
	t.Helper()
	out := map[string]ids.UUID{}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, filename FROM knowledge_document
			  WHERE managed_source = $1 AND archived_at IS NULL`, knowledge.HandbookSource)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			var filename string
			if err := rows.Scan(&id, &filename); err != nil {
				return err
			}
			out[filename] = id
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the filed handbook pages: %v", err)
	}
	return out
}
