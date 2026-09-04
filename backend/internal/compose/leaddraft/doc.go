// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package leaddraft writes an email to one lead, grounded in that lead's own
// record.
//
// It is a FOLD, not a drafter. The writer, the prompt, the voice floor, the
// deterministic fallback and the wire mapping are all persondraft's, and this
// package's whole job is to turn a lead into the Input that writer already
// takes. There were two full drafting implementations in this tree before this
// one — persondraft and accountdraft — and the difference between them is where
// the recipient comes from, which is not a reason to write a third.
//
// The shape is the one persondraft's own contract describes: the record IS the
// recipient, so the request carries nothing but optional steering. A lead is
// that shape exactly.
//
// **What a lead grounds less of.** A lead has no deal, no project and no
// claims — those hang off a contact, and a lead is by definition the record
// before a contact exists. The fold leaves those fields empty rather than
// reaching for a nearby account's, so a draft to a lead says less than a
// draft to a contact and nothing it cannot stand on.
//
// **It changes no record.** No pool here, no store with a write method: the
// guarantee is structural, the same way persondraft states it. The two writes
// further down are about the call rather than the lead — the AI usage meter
// and the model-call audit row.
//
// Everything but the caller's own intent is untrusted text — the lead's name,
// the company they wrote from, the body of a captured email — and rides inside
// persondraft's fence with the rest.
package leaddraft
