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
// Tables owned: docs/reference/entity-model/capture.md.
//
// capture_trace is the 24-hour diagnostic trace of what the pipeline decided
// about each message, swept rather than retained. capture_owner_identity holds
// a seat's other addresses. So mail among a colleague's own addresses is not
// read as correspondence, and an alias is never minted as a contact.
// capture_alias_sighting keeps one row per seat, address and message. So the
// two sightings the ladder demands cannot be one message counted twice.
// capture_import is one row per mailbox that delivered a message. An email is
// stored once, so a second importing seat's decisions about it live here. capture_sender_override is what a human decided about a
// sender, which the machine consults first and never writes over.
// capture_pending_counterparty is the open question about whose record an
// address is. Capture raises it, and the sweep raises it again for a contact the
// ceiling refused. The verdict engine resolves it.
// capture_test_mailbox_sent and capture_sweep_run are bookkeeping and not
// audited; a sweep receipt lets "when did it last succeed" outlive River's day
// of retention.
//
// capture_answered_sweep_cursor is where the pass publishing answered
// contacts stopped, so it reaches every stored contact. It is bookkeeping and
// not audited.
//
// Two more tables back the undo of an owner identity.
// capture_owner_identity_withdrawn holds a withdrawn identity for the ten
// minutes of its undo. capture_alias_refused holds a learned alias the seat
// withdrew, which discovery never claims again.
//
// Imports shared + platform only; never a sibling module.
package capture
