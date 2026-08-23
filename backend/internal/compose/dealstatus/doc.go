// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealstatus writes the deal page's one card: where the deal stands,
// what could lose it, and the one move to make next.
//
// It replaces three cards that stood beside each other and disagreed — a
// brief that restated records without reading them, a health score with no
// words, and a next-move box whose commonest branch read an open task back to
// the reader and then said it had nothing to add. Three partial answers to one
// question is not more information than one answer; it is a reader deciding
// which card to believe.
//
// ASSEMBLED UNDER THE CALLER'S ROW SCOPE. Every fact is read through its own
// module's gated read, so a record the caller cannot see never reaches a
// sentence. The cache key carries the reader's identity for the same reason:
// two readers with different grants never share a written card.
//
// CACHED ON THE FACTS, not on a clock. The key is a fingerprint over the
// assembled input plus the prompt and routing versions. A deal nobody touched
// costs one model call ever; a deal that just moved is rewritten BEFORE the
// request answers, so a card that arrives is current. An hourly refresh would
// be wrong in both directions — stale right after the meeting that mattered,
// and paying for a deal nobody opened.
//
// The one thing a fingerprint cannot bound is a deal whose facts churn on
// every read, so a floor holds the MODEL CALL to once per modelCallFloor per
// reader. Inside that floor a moved fingerprint still rewrites the card, just
// deterministically: the reader sees what changed, and generated_by says a
// composition wrote it. Serving the stale card instead would be the mistake an
// hourly refresh makes — describing the deal as it was before the thing that
// just happened.
//
// DEGRADES RATHER THAN FAILS. With no model lane, an exhausted budget, or a
// reply the grounding filter refuses, the same card is composed
// deterministically and generated_by names which wrote it. There is no blank
// AI card.
//
// EVERY SENTENCE IS CITED OR DROPPED. A sentence whose citations do not
// resolve is dropped whole rather than shown uncited.
package dealstatus
