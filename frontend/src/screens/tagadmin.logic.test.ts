// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import { nearMatches } from "./tagadmin.logic";

// A vocabulary drifts one near-miss at a time. The server refuses only an
// exact collision, which catches none of the ways a person actually creates a
// duplicate.

const vocabulary = [
  { id: "t-1", name: "K5 Conference" },
  { id: "t-2", name: "EV programme" },
  { id: "t-3", name: "Churn Risk" },
];

function names(proposed: string): string[] {
  return nearMatches(proposed, vocabulary).map((tag) => tag.name);
}

describe("warning about a near-duplicate word", () => {
  it("sees a word that differs only in case or spacing", () => {
    expect(names("  k5   conference ")).toEqual(["K5 Conference"]);
  });

  it("sees the separators a person reaches for instead of a space", () => {
    expect(names("k5-conference")).toEqual(["K5 Conference"]);
    expect(names("EV_programme")).toEqual(["EV programme"]);
  });

  // The same near-duplicate seen from its two ends: a longer name typed
  // against a shorter existing word, and a shorter one against a longer.
  it("sees a word that contains an existing one, and the reverse", () => {
    expect(names("K5 Conference 2026")).toEqual(["K5 Conference"]);
    expect(names("EV")).toEqual(["EV programme"]);
  });

  it("says nothing about a word that is genuinely new", () => {
    expect(names("Trade Fair")).toEqual([]);
  });

  // An empty box is not a near-match of everything.
  it("says nothing before a name is typed", () => {
    expect(names("   ")).toEqual([]);
  });
});
