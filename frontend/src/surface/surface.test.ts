// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The published frontend surface is a CONTRACT, and this is what keeps it one.
//
// Its Go counterpart is the marker-placement fitness test: there, the compiler
// makes internal/** unreachable and the test holds the rest. Here the compiler
// holds nothing — a bundler resolves whatever a path can reach — so the
// surface is exactly two things: this exports map, and the gate that refuses a
// unit importing past it. Widening the map is a reviewed act; widening it by
// accident, because some tool wanted a path, is the failure this test exists
// to make impossible.

import { describe, expect, it } from "vitest";
import pkg from "../../package.json" with { type: "json" };
import * as apiSurface from "./api";
import * as designSystem from "./design-system";
import * as appSurface from "./index";

describe("the published frontend surface", () => {
  it("publishes exactly three subpaths", () => {
    expect(Object.keys(pkg.exports).sort()).toEqual([
      "./api",
      "./app",
      "./design-system",
    ]);
  });

  // The names, not just the count. A re-export file that quietly lost a symbol
  // would still satisfy the map above while breaking every unit that imported
  // it — and a unit's screen is compiled in a lane a core-only change does not
  // run, so nothing else would notice until an installation did.
  it("publishes exactly these names", () => {
    expect(Object.keys(designSystem).sort()).toEqual([
      "Badge",
      "Button",
      // Callout, ChoiceList and TokenList: what a unit needs to WARN about a
      // choice, to offer one readably, and to draw the set that answers it.
      // Published together because they are one screen's worth of gap — a unit
      // ships no stylesheet, so each of the three was otherwise a hand-composed
      // approximation of a control the core already draws properly.
      "Callout",
      "Card",
      "ChoiceList",
      // DataTable: a unit had a primitive for one record's facts (FactList) and
      // none at all for a LIST of records — and a unit ships no stylesheet, so
      // the bare `<table>` it was left with drew unaligned and took the page
      // sideways with it. Not ListTable: that one is a record list with
      // server-backed dials a caller owes state to.
      "DataTable",
      "EmptyState",
      // FactList: a unit screen had no other way to draw a label→value pair,
      // because no extension ships a stylesheet — so both connector screens
      // hand-wrote a `<dl>` that nothing styled. See the note beside its export.
      "FactList",
      "Field",
      "RecordPicker",
      "SectionHeader",
      // SegmentedControl and TokenInput: a unit could offer a closed choice only
      // as a dropdown, and could collect a list only as comma-separated text.
      // Both already existed in core; neither was reachable from a unit.
      "SegmentedControl",
      "Select",
      "TextInput",
      "TokenInput",
      "TokenList",
    ]);
    expect(Object.keys(apiSurface).sort()).toEqual([
      "ProblemError",
      "QueryStates",
      "api",
      "isVersionSkew",
      "problemMessageOf",
      "throwProblem",
    ]);
    expect(Object.keys(appSurface).sort()).toEqual([
      "LocaleProvider",
      "formatDateTime",
      // formatNumber: `useT` refuses a raw number, so without it a unit's only
      // way to put a figure in a sentence is `String(n)` — ungrouped for every
      // reader. The narrowing and the formatter are one change.
      "formatNumber",
      "useCan",
      "useCanWrite",
      "useLocale",
      "useT",
    ]);
  });
});
