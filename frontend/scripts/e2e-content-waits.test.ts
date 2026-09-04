import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Playwright's LIST readers do not auto-wait: `allTextContents()`,
// `allInnerTexts()` and `all()` resolve against whatever matches at that
// instant and answer an empty result for zero matches rather than retrying.
// Placed right after a container's visibility assert, such a read is really
// asking "has the frame painted?" and then reports the body's absence as the
// body's CONTENTS being wrong.
//
// That misdirection is what this gate is for. The failure that prompted it
// named a specific German heading as missing, on a backend-only PR, so it was
// read as a content or locale regression; the drawer had simply not filled in
// yet. A flake that lies about its own cause is paid for by whoever reads the
// log next, and it had already been paid for twice.
//
// So the readers go through e2e/waits.ts, which waits for the content first.
// A spec that knows the number it expects should use neither and write
// `await expect(locator).toHaveCount(n)` — it waits, asserts and states the
// number in one step.
//
// ## What this gate does NOT see
//
// `count()` and `textContent()` have the same no-auto-wait behaviour and are
// deliberately not banned here, because their guarded and unguarded uses read
// identically: `await expect(sections.first()).toBeVisible()` followed by
// `sections.count()` is correct, and the count alone is not, and no rule over
// the text tells them apart. Banning them would buy a waiver on every correct
// site, which is a gate that trains people to waive it.
//
// The one shape of theirs worth naming, since this gate cannot: a bare
// `(await x.count()) === 0` used as a BRANCH condition fails by passing — an
// unpainted page takes the empty branch and the test reports success without
// having tested anything. e2e/waits.ts `signIn` is the fixed version, waiting
// for the page to resolve into one of its two states before asking which.

const here = dirname(fileURLToPath(import.meta.url));
const e2eDir = join(here, "..", "e2e");

// Derived from the tree: a spec added tomorrow is gated the day it lands.
function specFiles(): string[] {
  return readdirSync(e2eDir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".spec.ts"))
    .map((entry) => join(e2eDir, entry.name));
}

// A reader named in PROSE is not a reader in use. Block comments go entirely;
// a line comment only counts when it owns its line, so a `//` inside a URL
// cannot eat the code beside it. String literals go too, so a selector that
// happens to spell one of these names cannot trip the scan.
function code(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, " ")
    .replace(/^[ \t]*\/\/.*$/gm, " ")
    .replace(/(['"`])(?:\\.|(?!\1).)*\1/g, '""');
}

const READERS = /\.(allTextContents|allInnerTexts|all)\s*\(\s*\)/g;

function bareReadsIn(source: string): string[] {
  return [...code(source).matchAll(READERS)].map((match) => match[1]);
}

describe("e2e list reads wait for their content", () => {
  it("finds the reader it is looking for", () => {
    // The scan reports a count, and a matcher that had stopped matching
    // would report zero on a clean tree and PASS. This is the planted
    // positive that fails instead — including the two spellings the
    // pattern must not lose: a chained call and a scoped locator.
    expect(
      bareReadsIn("const xs = await page.locator('.h3').allTextContents();"),
    ).toEqual(["allTextContents"]);
    expect(bareReadsIn("const ls = await own.locator(SEL).all();")).toEqual([
      "all",
    ]);
    expect(
      bareReadsIn("const t = await page.getByRole('row').allInnerTexts();"),
    ).toEqual(["allInnerTexts"]);
  });

  it("does not read a reader named in prose or in a selector", () => {
    expect(bareReadsIn("// use textsOf, not .allTextContents()")).toEqual([]);
    expect(bareReadsIn("/* .all() is what this replaces */")).toEqual([]);
    expect(bareReadsIn('page.locator(".all()");')).toEqual([]);
  });

  it("reads every spec in the tree", () => {
    // Under-recognition is the one way this must not break: a scan that
    // walked the wrong directory would read nothing and report PASS.
    expect(specFiles().length).toBeGreaterThanOrEqual(15);
  });

  it("has no spec reading a list before it has rendered", () => {
    const offenders = specFiles().flatMap((path) => {
      const found = bareReadsIn(readFileSync(path, "utf8"));
      return found.map(
        (reader) =>
          `${relative(join(here, ".."), path)}: .${reader}() — read it through textsOf/itemsOf in e2e/waits.ts, or assert toHaveCount(n) on the locator`,
      );
    });
    expect(offenders).toEqual([]);
  });
});
