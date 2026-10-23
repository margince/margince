// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package roombrief writes the short brief on a deal's Deal Room tab: what the
// room's buyers did and what is waiting on the seller.
//
// Tables owned: none. A room moves with each buyer's click, so the brief is
// written on every request and stored nowhere.
//
// The input is read through the room's own gated reads, as the caller. So the
// brief only describes what that caller could open on the tab beside it.
//
// With no model lane, a spent budget, or a refused reply, the brief is a
// deterministic composition of the same records. generated_by names the writer.
//
// Every sentence is cited or dropped, under the grounding rule in
// compose/claims. A sentence citing a record the input did not carry is
// dropped whole.
package roombrief
