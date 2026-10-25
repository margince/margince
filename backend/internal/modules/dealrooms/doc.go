// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealrooms owns the buyer-facing Deal Room: one room per deal, the
// immutable releases that fix what a buyer was shown, the named contacts invited
// into it, the credentials that admit them, the documents the seller shares
// and the conversation both sides hold about them.
//
// THE ROOM IS A PROJECTION, NOT A SECOND CRM. Nothing here duplicates a record
// the deal already owns. The room points at a deal; a published release freezes
// the exact editorial text a buyer saw, so answering "what did they actually
// see in August?" never depends on what the deal says today. That is what makes
// a public edge safe to serve at all: it reads a release, never the live deal.
//
// The seller's half is the room, its lifecycle, its
// releases, and the contacts admitted to it. A room reads and writes through the
// SELLER's authority, which it takes from the parent deal — deal_room carries no
// owner of its own, so every read joins deal and applies that row-scope clause,
// and every write takes auth.EnsureWritable on the same deal on top.
//
// A BUYER IS NOT A SEAT. A participant is no app_user, consumes no licence and
// holds no CRM authority. A steward admits one, corrects them, reissues their
// credential and takes access away; all four are human-only, because deciding
// which outsider reads a deal's material is not a judgement an agent makes.
//
// The buyer's half is the credential exchange and the room-scoped session it
// opens (store_public*.go, handlers_public.go). The session is resolved again
// on every request, so access the seller withdrew stops at the next request.
// platform/auth admits a system principal without a check, so the buyer path
// never calls it. Each buyer store method carries the session's room as a SQL
// predicate instead. TestPublicHandlersReachOnlyTheSessionScopedStore and
// TestSessionScopedStoreNeverConsultsTheSeatGates hold that boundary.
//
// Tables owned: docs/reference/entity-model/dealrooms.md.
package dealrooms
