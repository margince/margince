// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package draftvoice is the one spelling of how a drafting surface writes in
// the SENDER's own voice.
//
// Three surfaces generate outbound email — the reply to an activity, the person
// composer, and account-started outbound — and only the reply drafter read the
// actor's Voice DNA. The other two composed draftrules.Shared and a comment
// claiming that block "carries the user's own voice instead". It does not:
// draftrules holds language, register, greeting and formatting rules, and not
// one sentence of how this particular person writes. So a rep who had built a
// voice profile got their own voice when they answered a mail and a generic one
// when they started a message from a contact's page — the same person, the same
// mailbox, two different writers.
//
// What lives here is everything a surface needs to draft under a profile:
// loading the actor's active one (Load), rendering it into the calling call's
// fence (Context.Block), and the deterministic anti-AI floor that decides
// whether the voiced draft may be served (Floor).
//
// **It reads and never writes.** The learning signals a served draft feeds back
// — RecordDraftedSignal, RejectDraft — are NOT here, because two of the three
// callers are packages built with no pool at all, and that zero-write guarantee
// is a dependency rather than a rule somebody remembers. The reply drafter keeps
// its own signal recording beside its store.
//
// Held by: TestEveryDraftingSurfaceLoadsTheSenderVoice
// (internal/compose/draftvoiceparity_test.go), which sweeps the tree for the
// surfaces composing draftrules.Shared and fails when one of them reaches a
// model without a voice seam.
package draftvoice
