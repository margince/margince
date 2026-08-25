// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Fitness function for a stylesheet a component BORROWS.
//
// `company360.css` defines the record page's row and card shapes. Nine screens
// draw those classes and only one imported the file; the rest rendered
// correctly because the company record page happens to pull it in for their
// sake. Mounted anywhere else — Storybook is such a place — the meta parts run
// together with no separator and the row name loses its link affordance:
//
//     Shopsystem-Migration — zweiter MandantQualifiedcloses 28/09/2026
//
// No unit test can see that, and on the record page it is invisible. It fails
// only when somebody reuses one of these components somewhere new, at which
// point it looks like their bug.
//
// The corpus comes from the DIRECTORY rather than a list kept here, so a screen
// that starts drawing one of these classes tomorrow is covered the day it is
// written. A hand-kept list is the shape of gate that reports PASS over a
// smaller tree than it thinks it is reading.
const dir = dirname(fileURLToPath(import.meta.url));

/** The stylesheet that defines the shared record shapes. */
const SHARED_SHEET = "company360.css";

/**
 * The classes only `company360.css` defines.
 *
 * Deliberately the ones with no definition anywhere else — a class a screen's
 * own stylesheet also declares is that screen's to own, and demanding the
 * import for it would be noise.
 */
const BORROWED = ["co-rowlink", "co-row-meta", "co-card"];

/** Every `.tsx` under screens/, excluding tests and stories. */
function sourceFiles(base: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(base, { withFileTypes: true })) {
    const path = join(base, entry.name);
    if (entry.isDirectory()) {
      out.push(...sourceFiles(path));
      continue;
    }
    if (
      entry.name.endsWith(".tsx") &&
      !entry.name.endsWith(".test.tsx") &&
      !entry.name.endsWith(".stories.tsx")
    ) {
      out.push(path);
    }
  }
  return out;
}

// A class name inside a className string, not merely the characters somewhere
// in the file: `co-row-meta` is a substring of nothing else here, but a prose
// mention in a comment is not a render and must not oblige an import.
function drawsBorrowedClass(source: string): boolean {
  const stripped = source
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");
  return BORROWED.some((name) =>
    new RegExp(`className=[^\\n]*\\b${name}\\b`).test(stripped),
  );
}

// The IMPORT, not the filename anywhere in the file. Every screen fixed by
// this rule carries a comment naming the stylesheet, so a substring test
// passes on the comment and the gate reports PASS over a file it never
// checked — which is exactly how the first draft of this test went green
// against a screen whose import had been deleted.
function importsSharedSheet(source: string): boolean {
  return new RegExp(`^\\s*import\\s+["'][^"']*${SHARED_SHEET}["'];`, "m").test(
    source,
  );
}

describe("a screen that draws the record's shared shapes imports them", () => {
  it("leaves no screen relying on a sibling to have loaded company360.css", () => {
    const offenders = sourceFiles(resolve(dir))
      .filter((path) => {
        const source = readFileSync(path, "utf8");
        if (!drawsBorrowedClass(source)) {
          return false;
        }
        return !importsSharedSheet(source);
      })
      .map((path) => path.slice(resolve(dir).length + 1));

    expect(
      offenders,
      `these screens draw ${BORROWED.join(", ")} without importing ${SHARED_SHEET}, ` +
        "so they render unstyled wherever the company record page has not " +
        `already loaded it — add 'import "./${SHARED_SHEET}";'`,
    ).toEqual([]);
  });

  // The gate is only worth its line if it can still SEE the defect. A census
  // that quietly matches nothing reports PASS forever.
  it("recognises the classes it is meant to police", () => {
    expect(drawsBorrowedClass('<span className="co-row-meta">')).toBe(true);
    expect(drawsBorrowedClass('<button className="co-rowlink x">')).toBe(true);
    // A mention in prose is not a render.
    expect(drawsBorrowedClass("// co-rowlink is defined elsewhere")).toBe(
      false,
    );
    // And the other half: a comment NAMING the stylesheet is not an import of
    // it. Every file this rule touches carries such a comment.
    expect(importsSharedSheet(`import "./${SHARED_SHEET}";`)).toBe(true);
    expect(importsSharedSheet(`// see ${SHARED_SHEET} for these`)).toBe(false);
  });
});
