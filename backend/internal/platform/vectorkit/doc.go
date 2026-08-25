// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package vectorkit holds the invariants every pgvector writer in this tree
// owes, in the pure form that has no table behind it: the zero-vector guard,
// the literal encoding, and the hash-plus-identity skip-compare.
//
// It exists because there are two writers. internal/modules/search owns the
// `embedding` table and holds its own copies of all three
// (search/embedding.go); internal/modules/knowledge owns knowledge_chunk and
// uses this package. That is a declared duplication, not an oversight — search
// predates this kit and refactoring it was deliberately kept out of the change
// that introduced knowledge, because widening that diff would have put a
// gated, contract-pinned module in the blast radius of a new feature.
// Folding search onto this kit is the follow-up.
//
// The SQL is deliberately NOT here. The two tables differ in shape and in what
// they join, and a shared query builder over them would be an abstraction with
// one caller each.
package vectorkit
