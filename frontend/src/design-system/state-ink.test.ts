// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  alternativesOf,
  appStylesheets,
  classesOf,
  colorValues,
  type Rule,
  rulesOf,
  selectorList,
  splitTopLevel,
  subjectOf,
} from "../../scripts/lib/css-rules";
import { states } from "./tokens-testing";

// A state's base is tuned to be SEEN. Text drawn in a state colour takes its
// `--<state>Text`, the member tokens.test.ts measures at 4.5:1 on every ground.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

const BASE_INK = new RegExp(`var\\(\\s*--(?:${states.join("|")})\\s*[,)]`);

// SVG geometry paints a shape and never a letter.
const GRAPHIC =
  /^(?:svg|path|circle|ellipse|rect|line|polyline|polygon|use|g)(?![\w-])/i;

// Boxes that hold one aria-hidden icon and no text, each with where that holds.
const GLYPH_TILES: Readonly<Record<string, string>> = {
  "capture-mark": "backfillrunview.tsx: an aria-hidden span around one icon",
  "capture-stat-glyph":
    "backfillrunview.tsx: an aria-hidden span around one icon",
  "unsub-done-mark": "unsubscribe.tsx: an aria-hidden span around one check",
  rfi: "the read-failure tile: a span around one icon, beside its own heading",
};

type Finding = Readonly<{ at: string; value: string; tile?: string }>;

function baseInked(all: readonly Rule[]): Finding[] {
  return all.flatMap((rule) => {
    const values = colorValues(rule.body).filter((value) =>
      BASE_INK.test(value),
    );
    if (values.length === 0) return [];
    return alternativesOf(rule.selector).flatMap((selector) => {
      const subject = subjectOf(selector);
      if (GRAPHIC.test(splitTopLevel(subject, ":")[0] ?? "")) return [];
      const tile = [...classesOf(subject)].find((name) => name in GLYPH_TILES);
      const at = `${relative(frontendRoot, rule.file)} ${selector}`;
      return values.map((value) => ({ at, value, tile }));
    });
  });
}

function probe(selector: string, body: string): Rule[] {
  return selectorList(selector).map((one) => ({
    file: join(frontendRoot, "probe.css"),
    selector: one,
    body,
  }));
}

describe("text in a state colour", () => {
  const sheets = appStylesheets(frontendRoot);
  const all = rulesOf(sheets);
  const found = baseInked(all);

  it("is lettered in the state's Text token, in every sheet the app ships", () => {
    expect(sheets.length).toBeGreaterThan(200);
    const textInks = all.filter((rule) =>
      colorValues(rule.body).some((value) =>
        new RegExp(`var\\(--(?:${states.join("|")})Text\\)`).test(value),
      ),
    );
    expect(textInks.length).toBeGreaterThan(70);
    expect(
      found
        .filter((finding) => finding.tile === undefined)
        .map(({ at, value }) => `${at} { color: ${value} }`),
    ).toEqual([]);
  });

  it("keeps every glyph-tile waiver on a rule that still needs it", () => {
    const used = new Set(found.map((finding) => finding.tile));
    expect(Object.keys(GLYPH_TILES).filter((name) => !used.has(name))).toEqual(
      [],
    );
  });

  it.each([
    [".p", "color: var(--warning);", 1],
    [".p:hover", "color: var(--danger);", 1],
    [":where(.q) .p", "color: var(--success);", 1],
    [":is(.p, .q)", "color: var(--info);", 2],
    [".p, .q svg", "color: var(--danger);", 1],
    [".p", "color: var(--warningText); color: var(--warning);", 1],
    [".p", "color: var(--danger, #c00);", 1],
    [".p", "color: color-mix(in srgb, var(--discovery) 80%, black);", 1],
    [".p svg", "color: var(--success);", 0],
    [".p > path:hover", "color: var(--danger);", 0],
    [".p", "color: var(--dangerText);", 0],
    [
      ".p",
      "background: var(--danger); border-color: var(--danger); --x-color: var(--danger);",
      0,
    ],
  ])("reads %s { %s } as %i base-inked text", (selector, body, count) => {
    expect(baseInked(probe(selector, body))).toHaveLength(count);
  });

  it("waives a named glyph tile and nothing that merely resembles one", () => {
    const [tile] = baseInked(probe(".capture-mark", "color: var(--success);"));
    expect(tile?.tile).toBe("capture-mark");
    const [text] = baseInked(
      probe(".capture-mark-label", "color: var(--success);"),
    );
    expect(text?.tile).toBeUndefined();
  });
});
