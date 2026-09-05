// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import type { WorklistItem } from "./worklist.queries";

// A row claims no verb it cannot perform.
//
// The rule is NOT "every row offers something": two rows deliberately offer
// nothing, because the queue can send a reader somewhere and cannot fix a rule,
// and a row the server named no verb for is still real work with nothing to
// press. Drawing a control on either would promise a repair this surface does
// not perform.
//
// What must hold is the other direction. Every verb the contract can send has
// to be ANSWERED somewhere — routed to a destination, or drawn inline by the
// component that owns it — or the row draws a button that goes nowhere, which
// is worse than no button at all.
//
// Nothing enforced that. `VERB_DESTINATION` is a partial map on purpose, so a
// tenth verb added to the contract would simply fall out of it and be silently
// undrawn, and the six answered inline are named nowhere. This is that list,
// and it fails rather than drifts.

const SCREENS = join(__dirname);

// Every verb the contract offers, read from the generated types rather than
// retyped: a census that names its own subjects proves only that they were
// named.
type Verb = WorklistItem["actions"][number];

// Where each verb is answered. ROUTED means VERB_DESTINATION carries it to the
// record the row is about; INLINE names the file that draws its control on the
// row itself. Exhaustive over the union, so a verb added to `crm.yaml` fails to
// compile here until somebody says where a reader answers it.
const ANSWERED_BY = {
  open: { how: "routed" },
  complete: { how: "routed" },
  snooze: { how: "routed" },
  // Answered on this page, so a link would send the reader where they already
  // are — except an introduction ask, which routes to the colleague's own
  // Network tab through decideDestination.
  decide: { how: "inline", file: "worklist.row.tsx" },
  merge: { how: "inline", file: "worklist.pair.tsx" },
  act: { how: "inline", file: "worklist.row.tsx" },
  set_aside: { how: "inline", file: "worklist.row.tsx" },
  dismiss: { how: "inline", file: "worklist.row.tsx" },
  acknowledge: { how: "inline", file: "worklist.row.tsx" },
} as const satisfies Record<
  Verb,
  { how: "routed" } | { how: "inline"; file: string }
>;

describe("a row claims no verb it cannot perform", () => {
  const row = readFileSync(join(SCREENS, "worklist.row.tsx"), "utf8");

  // The routed verbs, read from VERB_DESTINATION's own body. Read as source
  // rather than imported, because the map is not exported and exporting it to
  // satisfy a test would widen the module's surface for the test's convenience.
  //
  // SLICED TO THE MAP FIRST, so the pattern does not have to tell one top-level
  // arrow-valued map from another: VERB_LABEL sits beside it with the same
  // shape, and a census that told them apart by their parameter name would gain
  // members the day somebody renamed one.
  const map = row.match(/const VERB_DESTINATION\b[\s\S]*?\n};/)?.[0] ?? "";
  const routed = new Set(
    [...map.matchAll(/^ {2}(\w+): /gm)].map(([, verb]) => verb),
  );

  // A corpus that read NOTHING reports the same silence as one that found
  // nothing wrong, so the census says out loud that it found its subject.
  // Today three verbs are routed, so an empty parse would fail the arms below
  // by accident; that stops the day the last routed verb moves inline, which
  // the map's own comment anticipates.
  //
  // Asserted inside a test rather than at module scope: a bare expect there
  // aborts the file, and a suite reporting "no tests" reads as a configuration
  // problem rather than as this census losing the thing it audits.
  it("finds the destination map it reads as text", () => {
    expect(
      map,
      "VERB_DESTINATION is no longer a top-level const ending in `};` — this " +
        "census reads it as source and can no longer find it",
    ).not.toBe("");
    expect(
      routed.size,
      "the destination map parsed as empty, so this census read nothing " +
        "rather than found nothing",
    ).toBeGreaterThan(0);
  });

  it("routes exactly the verbs the destination map claims", () => {
    const declared = Object.entries(ANSWERED_BY)
      .filter(([, where]) => where.how === "routed")
      .map(([verb]) => verb)
      .sort();

    expect([...routed].sort()).toEqual(declared);
  });

  // THE CENSUS. A verb answered nowhere is a verb a reader is offered and
  // cannot use.
  it("answers every verb the contract can send", () => {
    const unanswered = Object.entries(ANSWERED_BY).filter(([verb, where]) => {
      if (where.how === "routed") {
        return !routed.has(verb);
      }
      // GUARDED, not merely mentioned. The named file has to ASK whether the
      // row offers this verb, in one of the two spellings the call sites use —
      // `offered("x")` through the helper, or `includes("x")` direct. A file
      // that merely contains the word satisfies nothing: deleting a control's
      // condition leaves the verb's name behind in the click handler, in
      // VERB_LABEL and in the prose, so a mention test stays green over a row
      // that draws nothing.
      const source = readFileSync(join(SCREENS, where.file), "utf8");
      return !(
        source.includes(`offered("${verb}")`) ||
        source.includes(`includes("${verb}")`)
      );
    });

    expect(
      unanswered.map(([verb]) => verb),
      "these verbs reach the row and nothing answers them, so a reader is " +
        "offered a control that does nothing",
    ).toEqual([]);
  });
});
