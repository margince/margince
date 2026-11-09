// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { en } from "./en";
import { PLURAL_SINGLE_KEY_DEBT } from "./plural-debt";

// One count, one key: the shape the census rule one-plural-rule cannot see.
// A value of `"{count} steps"` prints "1 steps" with no ternary and no second
// arm, so that rule has nothing to read.
//
// The subject is a key whose value interpolates `{count}` and has no
// `_one`/`_other` sibling. A narrower reading, a count followed by a bare `-s`
// noun, misses `{count} open tasks`, which prints "1 open tasks". "Any `-s` word
// after the count" matches `{count} converted, rates {date}`, where the count
// counts nothing. No pattern separates the two, so a narrowing must be a
// declared reason.
//
// So the corpus comes from the catalogue, cleared by the register in
// plural-debt.ts, which is closed and only shrinks.
describe("one count, one key", () => {
  const pluralSibling = (
    catalog: Readonly<Record<string, string>>,
    key: string,
  ): boolean =>
    [`${key}_one`, `${key}_other`, `${key}.one`, `${key}.many`].some(
      (sibling) => Object.hasOwn(catalog, sibling),
    );

  /** Every catalogue key that puts a count into a single message. */
  const singleKeyCounts = (
    catalog: Readonly<Record<string, string>> = en,
  ): string[] =>
    Object.entries(catalog)
      .filter(
        ([key, value]) =>
          typeof value === "string" &&
          value.includes("{count}") &&
          !key.endsWith("_one") &&
          !key.endsWith("_other") &&
          !key.endsWith(".one") &&
          !key.endsWith(".many") &&
          !pluralSibling(catalog, key),
      )
      .map(([key]) => key)
      .sort();

  it("registers every single-key count that was already here", () => {
    const found = singleKeyCounts();
    // Fail closed: a walk that reads an
    // empty catalogue finds no defect and reports PASS over all of them.
    expect(found.length).toBeGreaterThan(50);

    const unregistered = found.filter(
      (key) => !PLURAL_SINGLE_KEY_DEBT.has(key),
    );
    expect(
      unregistered,
      "A new catalogue key interpolates {count} into one message, so it prints " +
        "'1 steps' the day a reader reaches it with one. Give it a " +
        "<base>_one / <base>_other pair and read it through usePlural. " +
        "plural-debt.ts is CLOSED: it records what was already here and only " +
        "shrinks, so adding a line there is not the way out of this.",
    ).toEqual([]);
  });

  // An entry for a key since converted, renamed or deleted reads as a live
  // exemption and exempts nothing.
  it("keeps no register entry for a key the census no longer finds", () => {
    const found = new Set(singleKeyCounts());
    expect(
      [...PLURAL_SINGLE_KEY_DEBT.keys()].filter((key) => !found.has(key)),
      "A registered key is no longer a single-key count — converted, renamed " +
        "or gone. Delete its line: the register only shrinks, and this is it " +
        "shrinking.",
    ).toEqual([]);
  });

  it("flags a count in one key and passes a count split into two arms", () => {
    const planted = {
      "x.steps": "{count} steps",
      "x.tasks_one": "{count} task",
      "x.tasks_other": "{count} tasks",
    };
    expect(singleKeyCounts(planted)).toEqual(["x.steps"]);
  });

  // Every plural base carries both arms and no bare key. A missing arm leaves a
  // count without a sentence. A bare base beside the pair is a second answer.
  const brokenPluralPairs = (
    catalog: Readonly<Record<string, unknown>>,
  ): { bases: string[]; broken: string[] } => {
    const bases = [
      ...new Set(
        Object.keys(catalog)
          .filter((key) => /_(one|other)$/.test(key))
          .map((key) => key.replace(/_(one|other)$/, "")),
      ),
    ].sort();
    const broken = bases.filter(
      (base) =>
        !Object.hasOwn(catalog, `${base}_one`) ||
        !Object.hasOwn(catalog, `${base}_other`) ||
        Object.hasOwn(catalog, base),
    );
    return { bases, broken };
  };

  it("carries every plural base as a whole pair with no bare key", () => {
    const { bases, broken } = brokenPluralPairs(en);
    expect(bases.length).toBeGreaterThan(100);
    expect(
      broken,
      "A plural base is missing an arm or still has its bare key beside the " +
        "pair. Give it both <base>_one and <base>_other and delete <base>.",
    ).toEqual([]);
  });

  it("sees a half pair and a bare base beside a pair", () => {
    const planted = {
      "a.half_one": "{count} step",
      "b.bare": "{count} steps",
      "b.bare_one": "{count} step",
      "b.bare_other": "{count} steps",
      "c.whole_one": "{count} step",
      "c.whole_other": "{count} steps",
    };
    expect(brokenPluralPairs(planted).broken).toEqual(["a.half", "b.bare"]);
  });
});
