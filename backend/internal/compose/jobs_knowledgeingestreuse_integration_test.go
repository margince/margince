// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A re-ingest pays the embed lane only for passages whose text or binding moved.
// These tests drive the real worker over a document kept in step with its source.

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var syncedCorpus = knowledge.ManagedCorpus{Source: knowledge.GDocsSource, Name: "Google Docs", Topic: "Synced documents."}

// paragraph is one sentence repeated to about one passage's length, so each
// paragraph below is cut into a passage of its own.
func paragraph(sentence string) string { return strings.Repeat(sentence, 12) + "\n\n" }

var (
	policyOpening  = paragraph("The corpus answers only from what a workspace filed in it. ")
	policyMiddle   = paragraph("A second paragraph says something else entirely here. ")
	policyClosing  = paragraph("The third paragraph closes the page with its own words. ")
	policyReworded = paragraph("The third paragraph ends the page with other words now. ")

	policy        = policyOpening + policyMiddle + policyClosing
	policyRevised = policyOpening + policyMiddle + policyReworded
)

// countedIngest is the real worker over one document, counting what it sends the embed lane.
type countedIngest struct {
	*orphanIngest
	uploader context.Context
	embedder *gatedEmbedder
}

func newCountedIngest(t *testing.T) *countedIngest {
	t.Helper()
	e := integration.Setup(t)
	embedder := &gatedEmbedder{}
	return &countedIngest{
		orphanIngest: &orphanIngest{e: e, worker: newKnowledgeIngestWorker(e.Pool, blobstore.NewMemory(), embedder, slog.Default())},
		uploader:     corpusUploader(e),
		embedder:     embedder,
	}
}

// newSyncedIngest files body as a synced document and ingests it once.
func newSyncedIngest(t *testing.T, body string) *countedIngest {
	t.Helper()
	s := newCountedIngest(t)
	s.doc = s.file(t, body, knowledge.ExternalCreated)
	if sent := s.embedInputs(t); sent != len(knowledge.ChunkText(body)) {
		t.Fatalf("the first ingest sent %d passages to the embed lane, want every one", sent)
	}
	return s
}

func (s *countedIngest) file(t *testing.T, body string, want knowledge.ExternalOutcome) ids.UUID {
	t.Helper()
	doc, outcome, err := s.worker.store.UpsertExternal(s.uploader, syncedCorpus, knowledge.ExternalDocument{
		ExternalID: "drive-1", Filename: "Leave policy.md", ContentType: "text/markdown", Content: strings.NewReader(body),
	}, func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	if err != nil || outcome != want {
		t.Fatalf("filing = %v, %v; want %v", outcome, err, want)
	}
	return ids.UUID(doc.Id)
}

// embedInputs runs one ingest to done and returns how many passages it sent the
// embed lane.
func (s *countedIngest) embedInputs(t *testing.T) int {
	t.Helper()
	before := s.embedder.inputs.Load()
	s.mustIngest(t)
	return int(s.embedder.inputs.Load() - before)
}

// reopen leaves the document as an attempt that died after its embed step does,
// so the next run is River's retry of it.
func (s *countedIngest) reopen(t *testing.T) {
	t.Helper()
	if _, err := s.e.Pool.Exec(context.Background(),
		`UPDATE knowledge_document SET ingest_status = 'running' WHERE id = $1`, s.doc); err != nil {
		t.Fatalf("reopen the ingest: %v", err)
	}
}

// requireEveryPassageVectored fails unless each live passage carries a vector
// under the binding the embed lane answers with now.
func (s *countedIngest) requireEveryPassageVectored(t *testing.T) {
	t.Helper()
	identity, _ := s.embedder.EmbedIdentity()
	_, live := s.state(t)
	if got := s.e.WsCount(t, `SELECT count(*) FROM knowledge_chunk
		 WHERE document_id = $1 AND archived_at IS NULL AND embed_identity = $2`, s.doc, identity); got != live {
		t.Fatalf("%d of %d passages carry a %s vector", got, live, identity)
	}
}

func TestARetriedIngestOfAnUnchangedDocumentCallsTheEmbedLaneZeroTimes(t *testing.T) {
	s := newSyncedIngest(t, policy)

	s.reopen(t)
	if sent := s.embedInputs(t); sent != 0 {
		t.Fatalf("the retry sent %d unchanged passages to the embed lane, want 0", sent)
	}
	s.requireEveryPassageVectored(t)
}

func TestAReplacementThatRewordsOnePassageEmbedsOnlyThatPassage(t *testing.T) {
	s := newSyncedIngest(t, policy)
	if changed := newPassages(policy, policyRevised); changed != 1 {
		t.Fatalf("the fixture rewords %d passages, want exactly one", changed)
	}

	s.file(t, policyRevised, knowledge.ExternalReplaced)
	if _, live := s.state(t); live != 0 {
		t.Fatalf("the replaced bytes' %d passages are still live before the ingest", live)
	}
	if sent := s.embedInputs(t); sent != 1 {
		t.Fatalf("the re-ingest sent %d passages to the embed lane, want only the reworded one", sent)
	}
	s.requireEveryPassageVectored(t)
}

// A release whose page differs from the row's leaves the row's vectors on the
// passages it kept, whatever bytes around them moved.
func TestAHandbookPageAReleaseReplacesReusesTheVectorsOfItsKeptPassages(t *testing.T) {
	s := newCountedIngest(t)
	s.doc = s.boot(t, nil)[0]
	if sent := s.embedInputs(t); sent == 0 {
		t.Fatal("the first ingest sent nothing to the embed lane, so a reuse would prove nothing")
	}

	s.setChecksum(t, "sha256:an-earlier-release")
	if !slices.Contains(s.boot(t, nil), s.doc) {
		t.Fatal("the boot did not replace the page an earlier release filed")
	}
	if _, live := s.state(t); live != 0 {
		t.Fatalf("the replaced page's %d passages are still live before the ingest", live)
	}
	if sent := s.embedInputs(t); sent != 0 {
		t.Fatalf("the replacement's ingest sent %d kept passages to the embed lane, want 0", sent)
	}
	s.requireEveryPassageVectored(t)
}

func TestAChangedEmbeddingModelReEmbedsEveryPassage(t *testing.T) {
	s := newSyncedIngest(t, policy)

	s.embedder.identity = "fake/other@8"
	s.reopen(t)
	if sent, want := s.embedInputs(t), len(knowledge.ChunkText(policy)); sent != want {
		t.Fatalf("under a new model the re-ingest sent %d of %d passages to the embed lane", sent, want)
	}
	s.requireEveryPassageVectored(t)
}

// newPassages counts the passages of after whose text before does not hold.
func newPassages(before, after string) int {
	held := map[string]bool{}
	for _, c := range knowledge.ChunkText(before) {
		held[c.Text] = true
	}
	n := 0
	for _, c := range knowledge.ChunkText(after) {
		if !held[c.Text] {
			n++
		}
	}
	return n
}
