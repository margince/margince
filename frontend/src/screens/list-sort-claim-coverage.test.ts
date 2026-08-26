// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Fitness function for a list that claims an ordering it cannot have.
//
// `/partners` accepts a `sort` parameter and its handler never reads it: the
// store orders by organization id, which is its keyset. The screen opened on a
// tab labelled "Newest", sent `sort=-created_at`, and drew rows in uuid order.
// Nothing failed. The reader was simply told an ordering the list did not have,
// which is worse than being told none — a wrong order looks like data, and a
// missing one looks like a missing control.
//
// The tell is textual and unambiguous: a screen whose list fetcher does not put
// `sort` on the wire has no ordering to offer, so it may not name one either —
// not as `initialSort`, not as a view tab carrying `sort:`, and not through
// `standardViews`, whose tabs order by `-created_at` unless told otherwise.
//
// The census IS the directory, so a new list screen is covered the day it is
// written rather than the day somebody remembers this file. It is the same
// derivation `list-page-size-coverage.test.ts` makes over the same corpus, and
// for the same reason: the four screens that gate caught had each been missed
// by a hand-kept list.
//
// WHAT THIS DOES NOT CATCH, deliberately: the other direction — a screen that
// sends `sort` while declaring no sortable column, so the dial exists and
// nothing can reach it. Telling a COLUMN's `sort:` from a view tab's needs the
// type checker plus a rule for the shared column helpers in `recordlist.tsx`,
// which declare theirs in another file; a gate whose rules are that fiddly is
// one that gets worked around rather than fixed. That half is stated as a rule
// in `design-system/README.md` instead, where the next author reads it.
const dir = dirname(fileURLToPath(import.meta.url));

/** Every screen that reads a list through the shared wrapper. */
function listScreens(): string[] {
  return readdirSync(dir)
    .filter((file) => file.endsWith(".tsx"))
    .filter((file) => !file.includes(".test.") && !file.includes(".stories."))
    .filter((file) => {
      const source = readFileSync(resolve(dir, file), "utf8");
      // Calls it, rather than declaring it: listquery.tsx is the wrapper itself
      // and owns no fetcher of its own.
      return (
        /\buseListQuery</.test(source) &&
        !/export function useListQuery/.test(source)
      );
    })
    .sort();
}

/** Does this screen's list fetcher put `sort` on the wire? */
function sendsSort(source: string): boolean {
  return /\n\s*sort: query\.sort \|\| undefined,/.test(source);
}

/** Every way this screen names an ordering to the reader. */
function sortClaims(source: string): string[] {
  const claims: string[] = [];
  if (/\n\s*initialSort:/.test(source)) {
    claims.push("initialSort");
  }
  // A view tab's own sort. `label:` beside it is what tells a tab from the
  // fetcher's `sort: query.sort` and from a column's field name.
  if (/\{\s*label: [^}]*\bsort: "/.test(source)) {
    claims.push("a view tab carrying sort:");
  }
  // Its tabs order by -created_at unless the caller names another sort, so
  // calling it is a claim whatever the arguments say.
  if (/\bstandardViews\(/.test(source)) {
    claims.push("standardViews");
  }
  return claims;
}

describe("a list claims no ordering its endpoint cannot give", () => {
  it("finds the list screens rather than trusting a list written here", () => {
    // The census IS the directory. A hand-kept list would be the thing that
    // went stale, which is the failure this test exists to prevent.
    expect(listScreens()).toContain("people.tsx");
    expect(listScreens()).toContain("partners.tsx");
    expect(listScreens().length).toBeGreaterThanOrEqual(6);
  });

  it.each(listScreens())("%s", (file) => {
    const source = readFileSync(resolve(dir, file), "utf8");
    if (sendsSort(source)) {
      return;
    }
    expect(
      sortClaims(source),
      `${file}'s list fetcher sends no sort, so the server orders these rows ` +
        `its own way — naming an ordering anyway tells the reader something ` +
        `the list does not do. Either put sort on the wire or drop the claim.`,
    ).toEqual([]);
  });
});
