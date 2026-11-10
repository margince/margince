// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The knowledge capability of the extension Runtime: a unit files the documents
// it reads from its provider into the corpus the core keeps for it.
//
// What a unit gets is a door onto the knowledge store's own filing, not a
// second path around it. The corpus is the unit's, found by the unit's name in
// the table below, so there is no corpus argument to get wrong. The accepted
// types, the document size ceiling, the per-corpus passage ceiling and the
// ingest queue are the upload route's own. The write runs on the invocation's
// principal: a job tick as the job, a request as the member who made it, whose
// knowledge_document grant is then checked as it is for an upload.

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/pkg/extension"
)

// extensionCorpora is the units the core keeps a corpus for. The key is the unit
// name and the corpus's managed_source is the same word, which the schema's
// CHECK also lists: adding a unit here is a migration too.
var extensionCorpora = map[string]knowledge.ManagedCorpus{
	"gdocs": {
		Source: knowledge.GDocsSource,
		Name:   "Google Docs",
		Topic: "Company documents kept in Google Docs: policies, procedures and rules, " +
			"with the owner, department, country and effective date each one states.",
	},
}

// BindExtensionKnowledge records the object store a unit's documents are kept
// in. A role that serves or runs a unit that files documents calls it once at
// boot; one that does not leaves UpsertDocument refusing by name. It is a
// separate bind for the reason BindExtensionCapture is: it is a deployment
// dependency, not derivable from the pool.
func BindExtensionKnowledge(blob blobstore.Store) {
	extensionRuntimeDeps.mu.Lock()
	defer extensionRuntimeDeps.mu.Unlock()
	extensionRuntimeDeps.knowledgeBlob = blob
}

// errKnowledgeUnwired is a role that bound no object store.
var errKnowledgeUnwired = errors.New("compose: this role bound no object store for extension knowledge")

//nolint:ireturn // the published port IS the seam.
func (r *callRuntime) Knowledge() extension.Knowledge { return runtimeKnowledge{rt: r} }

type runtimeKnowledge struct{ rt *callRuntime }

func (k runtimeKnowledge) store(ctx context.Context) (context.Context, *knowledge.Store, knowledge.ManagedCorpus, error) {
	ctx, err := k.rt.scoped(ctx)
	if err != nil {
		return nil, nil, knowledge.ManagedCorpus{}, err
	}
	corpus, ok := extensionCorpora[k.rt.unit]
	if !ok {
		return nil, nil, corpus, extension.ErrNoKnowledge
	}
	if k.rt.deps.pool == nil || k.rt.deps.knowledgeBlob == nil {
		return nil, nil, corpus, errKnowledgeUnwired
	}
	return ctx, knowledge.NewStore(InstallationDB(k.rt.deps.pool)).WithBlobstore(k.rt.deps.knowledgeBlob), corpus, nil
}

func (k runtimeKnowledge) UpsertDocument(ctx context.Context, d extension.KnowledgeDocument) (extension.KnowledgeResult, error) {
	ctx, store, corpus, err := k.store(ctx)
	if err != nil {
		return extension.KnowledgeResult{}, err
	}
	if limit := DefaultUploadLimits().KnowledgeDocument; int64(len(d.Body)) > limit {
		return extension.KnowledgeResult{}, fmt.Errorf("%w: the document is %d bytes and the ceiling is %d",
			extension.ErrInvalid, len(d.Body), limit)
	}
	inserter, err := extensionJobInserter(k.rt.deps.pool)
	if err != nil {
		return extension.KnowledgeResult{}, fmt.Errorf("compose: opening the queue for the document's ingest: %w", err)
	}
	doc, outcome, err := store.UpsertExternal(ctx, corpus, knowledge.ExternalDocument{
		ExternalID: d.ExternalID, Filename: d.Filename, ContentType: d.ContentType,
		Content: bytes.NewReader(d.Body), Meta: d.Meta,
	}, knowledgeIngestEnqueue(inserter))
	if err != nil {
		return extension.KnowledgeResult{}, knowledgeRefusal(err)
	}
	return extension.KnowledgeResult{DocumentID: doc.Id.String(), Outcome: extension.KnowledgeOutcome(outcome)}, nil
}

func (k runtimeKnowledge) DeleteDocument(ctx context.Context, externalID string) error {
	ctx, store, corpus, err := k.store(ctx)
	if err != nil {
		return err
	}
	return knowledgeRefusal(store.DeleteExternal(ctx, corpus.Source, externalID))
}

// knowledgeRefusal maps what the store refuses a filing for onto the published
// vocabulary, so a unit tells "your input" from "try later" without importing a
// core type.
func knowledgeRefusal(err error) error {
	var unsupported *knowledge.UnsupportedTypeError
	var filed *knowledge.AlreadyFiledError
	var full *knowledge.CorpusFullError
	var busy *knowledge.IngestBusyError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &busy):
		return fmt.Errorf("%w: %w", extension.ErrKnowledgeBusy, err)
	case errors.As(err, &unsupported), errors.As(err, &filed), errors.As(err, &full):
		return fmt.Errorf("%w: %w", extension.ErrInvalid, err)
	default:
		return err
	}
}

func (k runtimeKnowledge) ExternalIDs(ctx context.Context) ([]string, error) {
	ctx, store, corpus, err := k.store(ctx)
	if err != nil {
		return nil, err
	}
	return store.ExternalIDs(ctx, corpus.Source)
}
