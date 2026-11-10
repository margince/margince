// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The knowledge capability is part of the published extension surface.
//
//margince:extension-surface

package extension

// A unit that reads documents from somewhere else puts them in the corpus the
// core keeps for it. The core files them exactly as an upload is filed: the same
// accepted types, the same size ceiling, the same per-corpus passage ceiling,
// the same ingest queue. A unit chooses none of those and cannot get past them.

import (
	"context"
	"errors"
)

// ErrNoKnowledge answers a unit the core keeps no corpus for. The core decides
// which units may file documents; a unit cannot widen that by naming a corpus,
// because it names none.
var ErrNoKnowledge = errors.New("extension: the core keeps no knowledge corpus for this unit")

// ErrKnowledgeBusy answers a replacement refused because the earlier version of
// the document is being read right now. Retry on the next run.
var ErrKnowledgeBusy = errors.New("extension: the document is being read; retry the replacement later")

// KnowledgeOutcome says what a filing did.
type KnowledgeOutcome string

// The three things UpsertDocument can do.
const (
	KnowledgeCreated   KnowledgeOutcome = "created"
	KnowledgeReplaced  KnowledgeOutcome = "replaced"
	KnowledgeUnchanged KnowledgeOutcome = "unchanged"
)

// KnowledgeDocument is one source document.
type KnowledgeDocument struct {
	// ExternalID is the id the source gave the document (a Drive file id). Filing
	// the same id again replaces the document.
	ExternalID string
	// Filename is what a citation calls the document.
	Filename string
	// ContentType is text/markdown, text/plain, text/csv or application/json.
	ContentType string
	// Body is the document's text. The core refuses one over its upload ceiling.
	Body []byte
	// Meta is what the source said about the document: at most 16 fields, each
	// name 32 characters and each value 500. A field the source did not state is
	// left out, never guessed. It is shown beside a citation.
	Meta map[string]string
}

// KnowledgeResult says which document the filing landed on.
type KnowledgeResult struct {
	DocumentID string
	Outcome    KnowledgeOutcome
}

// Knowledge files documents in the calling unit's own corpus. Like every other
// capability it is reached through the Runtime and ends with the call.
type Knowledge interface {
	// UpsertDocument creates the document, or replaces the live one with the same
	// ExternalID. Identical bytes do nothing (KnowledgeUnchanged), so a sync that
	// offers every document on every run costs one hash each. A replacement
	// swaps the passages: the document goes back to queued and the ingest that
	// follows reads the new text.
	UpsertDocument(ctx context.Context, d KnowledgeDocument) (KnowledgeResult, error)

	// DeleteDocument removes the document with this ExternalID, its passages and
	// its stored file. One already gone is not an error.
	DeleteDocument(ctx context.Context, externalID string) error

	// ExternalIDs lists the ExternalID of every live document in the unit's
	// corpus, so a sync can retire what its source no longer offers.
	ExternalIDs(ctx context.Context) ([]string, error)
}
