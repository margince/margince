// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package knowledge owns the document corpus: a named body of uploaded text a
// person can ask free-text questions of, answered only from what is in it.
//
// It holds its own vectors rather than joining search's `embedding` table.
// That is a deliberate separation: search's embeddable-entity registry is the
// same list as its SEARCHABLE-entity registry (searchBranches), so a corpus
// chunk added there would appear in every user's cross-object search results,
// in the context-anchor enum and in the query vocabulary. A corpus is asked,
// not browsed.
//
// The ask never calls search. It is one corpus, one lane, no fusion.
//
// # Tables owned
//
// knowledge_corpus, knowledge_document, knowledge_chunk.
//
// A chunk is a derived artifact of its document and carries no audit identity
// of its own — the document row is its history. That is not a convenience: an
// audit image is append-only and outlives the delete that clears what it
// quotes, so a chunk of document text in one would stay readable through field
// history, record history and the compliance log after the document was
// deleted. The document's own image is pinned to four keys that are not the
// document's prose, and a test holds both halves of that.
//
// # What erasure here does and does not reach
//
// Deleting a document destroys its chunks, its vectors and its stored file, and
// tombstones what it destroyed so the audit images left behind are bounded. A
// vector is not claimed to be anonymous: it is lossy but derived from the prose,
// and a similarity probe reconstructs neighbourhoods of what was erased — the
// tree already holds that position, registering `embedding` as purged but never
// exported.
//
// The stated limit: erasing a document is how corpus prose is reached. A subject
// named inside a third party's uploaded notes is a business decision to delete
// that document, not an Art. 17 obligation this product discharges
// automatically, and these tables are deliberately not registered as
// PII-bearing. A stated limit beats an implied guarantee, which is why it is
// written here rather than only in a design note.
//
// Imports shared + platform only; never a sibling module.
package knowledge
