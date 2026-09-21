// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every CSS rule under a tree, and the three questions a selector is asked
// about it. Read as text rather than through a parser: the sheets here are
// hand-written and flat, and the whole corpus is one regex pass.

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

export function stylesheets(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : stylesheets(path);
    }
    return path.endsWith(".css") ? [path] : [];
  });
}

export type Rule = { file: string; selector: string; body: string };

export function rules(root: string): Rule[] {
  const sheets = stylesheets(root);
  if (sheets.length === 0) {
    throw new Error(
      `no stylesheet under ${root} — a rule walk over none reports a clean tree`,
    );
  }
  const all: Rule[] = [];
  for (const file of sheets) {
    const sheet = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    // Innermost brace pairs, so a rule nested in an @media is found as
    // itself and the query around it never matches as a selector.
    for (const [, selector, body] of sheet.matchAll(/([^{}]*)\{([^{}]*)\}/g)) {
      for (const one of selector.split(",")) {
        const trimmed = one.trim();
        if (trimmed) all.push({ file, selector: trimmed, body });
      }
    }
  }
  return all;
}

// A functional pseudo-class names something OTHER than the element it is
// written on, so its argument is not part of that element's classes:
// `:not(.btn)` would otherwise make `btn` a class the chip carries, and the
// subtree search would then find nothing at all.
export function classesOf(compound: string): Set<string> {
  const bare = compound.replace(/:[\w-]+\([^)]*\)/g, "");
  return new Set([...bare.matchAll(/\.([\w-]+)/g)].map(([, name]) => name));
}

export function compounds(selector: string): string[] {
  return selector.split(/[\s>+~]+/).filter(Boolean);
}

// The SUBJECT of a selector: the compound the rule actually paints, which is
// the last one. `.palette-row .type` styles the chip, not the row — reading
// its first compound instead put every `.palette-row` descendant inside a
// chip it is only a sibling of.
export function subjectClasses(selector: string): Set<string> {
  const parts = compounds(selector);
  return classesOf(parts[parts.length - 1] ?? "");
}

// The custom properties a rule's body sets as its INK.
//
// `(?:^|[;{\s])` is what keeps this off --*-color: the character before a
// longhand's `color:` is always a hyphen.
export function inks(body: string): string[] {
  return [...body.matchAll(/(?:^|[;{\s])color:\s*var\((--[\w-]+)\)/g)].map(
    ([, ink]) => ink,
  );
}
