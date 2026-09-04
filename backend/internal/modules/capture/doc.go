// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capture owns inbound capture (interfaces.md §1): the ONE
// connector.Sink implementation — a connector normalizes provider
// records, capture writes them. One transaction per record: raw
// original + domain row + audit entry (connector principal, never
// forgeable) + the captured event through the outbox, idempotent on
// the (source_system, source_id) natural key so replays are free.
//
// The registry holds the compiled-in connector set and enforces the
// grant-time scope intersection: a connector's declared scopes must be
// ⊆ the granting human's — connector ≤ human, exactly like agents.
//
// Tables owned: raw_capture, capture_connection, capture_trace (the
// 24-hour diagnostic trace of what the pipeline decided about each
// message, swept rather than retained), capture_exclusion (the addresses
// and domains the sink refuses before any write), capture_owner_identity
// (a seat's OTHER addresses, so mail among a person's own addresses is
// not read as correspondence and an alias is never minted as a contact),
// capture_import (one row per mailbox that delivered a message — an email
// is stored once, so this is the only place a second importing seat's
// decisions about it can live), capture_counterparty_hold (whose mail one
// seat keeps out of the shared timeline) and capture_thread_verdict (what a
// classifier concluded about one thread, for one seat) and
// capture_sender_override (what a person decided about a sender, which the
// machine consults first and never writes over) and
// capture_pending_counterparty (the open question about whose record an
// address is — raised at capture, re-raised by the sweep for a contact the
// ceiling refused, and resolved by the verdict engine).
// Imports shared + platform only; never a sibling module.
package capture
