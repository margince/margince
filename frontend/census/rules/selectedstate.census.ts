// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A selected, pressed or current state says where the reader is, so it is
// neutral like a pressed filter pill (filterpills.css). The accent is the brand
// and the primary action, so a state in it reads as the control to press.
// A control's value (`:checked`, `aria-checked`) and a progress rung keep it.
// The rail's current row marks the app's location, not a pick among peers.

import { relative } from "node:path";
import { alternativesOf, type Rule } from "../../scripts/lib/css-rules";
import { type CensusRule, frontendRoot, isAppSheet, sheetRules } from "../tree";

export const MARKERS = {
  on: /\.on(?![\w-])/,
  active: /\.active(?![\w-])/,
  "is-selected": /\.is-selected(?![\w-])/,
  "-selected": /\.[\w-]+-selected(?![\w-])/,
  "aria-pressed": /\[aria-pressed="true"\]/,
  "aria-current": /\[aria-current(?:[\]=])/,
  "aria-selected": /\[aria-selected="true"\]/,
} as const;

// `.row:not(.on)` paints the rows that are not selected.
const NEGATION = /:not\((?:[^()]|\([^()]*\))*\)/g;

// Every property that puts a colour on the element: its ground, its ink, its
// edge and an inset bar drawn as a shadow.
const PAINT =
  /^(?:background(?:-color)?|color|border(?:-[a-z-]+)?|box-shadow)$/;
const ACCENT = /var\(\s*--accent/;

// Whole entries, so a new selector, sheet or property stays a finding.
export const LOCATION_MARKERS = new Set([
  "src/app/shell.css: .rail .navitem.active { background: var(--accentLight) }",
  "src/app/shell.css: .rail .navitem.active { color: var(--accentText) }",
  "src/app/shell.css: .rail .railmore.active { color: var(--accentText) }",
  "src/app/shell.css: .rail .railmore.active { background: var(--accentLight) }",
  "src/app/shell.css: .rail.collapsed:not(.menuopen) .navitem.active { box-shadow: inset 0 0 0 1px var(--accentText) }",
  "src/app/shell.css: .rail > .navlevel .navitem.active { box-shadow: inset 0 0 0 1px var(--accentText) }",
  "src/app/shell.css: .rail:not(.menuopen) .railmore.active { box-shadow: inset 0 0 0 1px var(--accentText) }",
]);

export function stateMarkers(selector: string): string[] {
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

export function accentStates(rules: readonly Rule[]): string[] {
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

export function unexcused(rules: readonly Rule[]): string[] {
  return accentStates(rules).filter((found) => !LOCATION_MARKERS.has(found));
}

export const rule: CensusRule = {
  name: "selectedstate",
  corpus: (tree) => tree.files.filter(isAppSheet),
  // A location marker the tree no longer has is a pattern that went blind. So
  // is a state marker that matches nothing. Either would pass every state it
  // misses.
  floors: (corpus) => {
    const rules = sheetRules(corpus);
    const found = new Set(accentStates(rules));
    const seen = new Set(
      rules.flatMap((one) =>
        alternativesOf(one.selector).flatMap(stateMarkers),
      ),
    );
    return [
      ...[...LOCATION_MARKERS]
        .filter((entry) => !found.has(entry))
        .map((entry) => `the rail location marker is gone: ${entry}`),
      ...Object.keys(MARKERS)
        .filter((name) => !seen.has(name))
        .map((name) => `no rule in the tree carries the ${name} state marker`),
    ];
  },
  across: (corpus) => unexcused(sheetRules(corpus)),
  advice:
    "A selected, pressed or current state is neutral: paint it from no --accent token.",
};
