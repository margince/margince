// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  alternativesOf,
  appStylesheets,
  type Rule,
  rulesOf,
} from "../../scripts/lib/css-rules";

// A selected, pressed or current state says where the reader is, so it takes
// the pressed filter pill's neutral pair (filterpills.css). The accent is the
// brand and the primary action (README), and a state drawn in it reads as the
// one control worth pressing. A control's VALUE (`:checked`, a switch's
// `aria-checked`) and an open disclosure are not where the reader is, and are
// left to the accent the platform paints them in.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

const MARKERS = {
  on: /\.on(?![\w-])/,
  active: /\.active(?![\w-])/,
  "is-selected": /\.is-selected(?![\w-])/,
  "aria-pressed": /\[aria-pressed="true"\]/,
  "aria-current": /\[aria-current(?:[\]=])/,
  "aria-selected": /\[aria-selected="true"\]/,
} as const;

// `.row:not(.on)` paints the rows that are NOT selected.
const NEGATION = /:not\((?:[^()]|\([^()]*\))*\)/g;

// Every property that puts a colour on the element: its ground, its ink, its
// edge and an inset bar drawn as a shadow.
const PAINT =
  /^(?:background(?:-color)?|color|border(?:-[a-z-]+)?|box-shadow)$/;
const ACCENT = /var\(\s*--accent/;

function stateMarkers(selector: string): string[] {
  const positive = selector.replaceAll(NEGATION, "");
  return Object.entries(MARKERS)
    .filter(([, marker]) => marker.test(positive))
    .map(([name]) => name);
}

function accentPaint(body: string): string[] {
  return body
    .split(";")
    .map((declaration) => declaration.trim())
    .filter((declaration) => {
      const colon = declaration.indexOf(":");
      if (colon < 0) return false;
      const property = declaration.slice(0, colon).trim();
      return PAINT.test(property) && ACCENT.test(declaration.slice(colon + 1));
    });
}

function accentStates(rules: readonly Rule[]): string[] {
  return rules.flatMap((rule) =>
    alternativesOf(rule.selector)
      .filter((selector) => stateMarkers(selector).length > 0)
      .flatMap((selector) =>
        accentPaint(rule.body).map(
          (declaration) =>
            `${relative(frontendRoot, rule.file)}: ${selector} { ${declaration} }`,
        ),
      ),
  );
}

describe("a selected or pressed state is neutral", () => {
  const rules = rulesOf(appStylesheets(frontendRoot));

  it("paints no selected, pressed or current state from an --accent token", () => {
    expect(accentStates(rules)).toEqual([]);
  });

  // A marker that matches nothing in the tree is a pattern that went blind,
  // and the check above would then pass over every state it no longer sees.
  it("finds a rule in the tree for every state marker it reads", () => {
    const seen = new Set(
      rules.flatMap((rule) =>
        alternativesOf(rule.selector).flatMap(stateMarkers),
      ),
    );
    expect([...seen].sort()).toEqual(Object.keys(MARKERS).sort());
  });

  it("fails a new state selector the tree does not have yet", () => {
    const probe = (selector: string, body: string): Rule[] => [
      { file: join(frontendRoot, "src", "probe.css"), selector, body },
    ];
    expect(
      accentStates(
        probe(
          '.steps a[aria-current="step"]',
          "background: var(--accentLight)",
        ),
      ),
    ).toHaveLength(1);
    expect(
      accentStates(probe(".tabs .tab.on .glyph", "color: var(--accentText)")),
    ).toHaveLength(1);
    expect(
      accentStates(
        probe(".row.is-selected", "box-shadow: inset 3px 0 var(--accent)"),
      ),
    ).toHaveLength(1);
    expect(
      accentStates(probe(".row:not(.on)", "color: var(--accentText)")),
    ).toEqual([]);
    expect(
      accentStates(probe(".row.on", "outline-color: var(--accent)")),
    ).toEqual([]);
  });
});
