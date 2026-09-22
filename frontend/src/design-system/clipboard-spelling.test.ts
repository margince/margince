// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  parseSource,
} from "../../scripts/lib/source-tree";

// Asking the browser to copy has ONE spelling, and it is `useClipboardCopy`.
//
// Eight screens wrote it out by hand before this gate existed, and the reason
// they diverged is not that anyone disagreed — each author solved it in the
// file they were in, correctly for that file, and never saw the other seven.
// Three guarded the API object, one did not, one guarded it and then returned
// in silence, and the notice was a `Callout` on five, a tinted `span` on one, a
// bare paragraph on another and nothing at all on the last. So a reader on a
// plain-http installation learned a different thing, or nothing, depending on
// which screen they happened to be standing on.
//
// What makes this worth a gate rather than a cleanup is the failure's SHAPE:
// `navigator.clipboard` is undefined outside a secure context, so the missing
// capability throws on the property access rather than rejecting, and the
// `.catch` an author reasonably reaches for never runs. Getting that right is
// not obvious, it is invisible in every environment a developer tests in, and
// it has to be got right at every site. One site is the only number that scales.
//
// The subject is the ACCESS, not the import: a file that never mentions
// `useClipboardCopy` but reaches for `navigator.clipboard` is exactly the
// regression, and one that reaches for it through a local alias
// (`const writer = navigator.clipboard`) is the same offence — which is why the
// walk matches the property access itself rather than a call to `writeText`.

const frontendRoot = resolve(__dirname, "..", "..");
const repoRoot = resolve(frontendRoot, "..");

// The two files that are ALLOWED to name it: the one implementation, and the
// helper that installs a stub for it in tests. Spelled as paths relative to the
// repo so a move shows up here as a failure rather than as silence.
const OWNERS: ReadonlySet<string> = new Set([
  "frontend/src/design-system/clipboardcopy.tsx",
  "frontend/src/design-system/clipboard-testing.ts",
]);

/**
 * Whether a file can possibly carry the offence, decided without parsing it.
 *
 * SOUND rather than clever, and that is the whole argument for having it: the
 * matcher below fires on a property whose name is the identifier `clipboard`,
 * and an identifier has to appear literally in the text for the parser to build
 * that node. So every file the matcher could flag contains this substring, and
 * the filter is strictly WIDER than the thing it stands in front of.
 *
 * It is here because it was measured, not because it looked faster: parsing all
 * 2251 files costs ~1.9s on a quiet machine and timed out past 10s on a loaded
 * four-way CI shard, while 28 files carry the word at all — ~320ms. A gate that
 * reds the build on how busy the runner is teaches everyone to re-run it.
 */
function mayReachForTheClipboard(text: string): boolean {
  return text.includes("clipboard");
}

/** Every line in `source` that reaches for `navigator.clipboard`. */
function clipboardAccessesIn(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node) => {
    // `navigator.clipboard` and `window.navigator.clipboard` alike: the subject
    // is a property named `clipboard` hanging off something named `navigator`.
    if (
      ts.isPropertyAccessExpression(node) &&
      node.name.text === "clipboard" &&
      node.expression.getText(source).endsWith("navigator")
    ) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart()).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return lines;
}

function sitesAcrossTheTree(): string[] {
  const files = [
    ...filesUnder(join(frontendRoot, "src")),
    ...extensionFrontendFiles(join(repoRoot, "extensions")),
  ];
  return files.flatMap((path) => {
    const where = relative(repoRoot, path);
    if (OWNERS.has(where)) return [];
    const text = readFileSync(path, "utf8");
    if (!mayReachForTheClipboard(text)) return [];
    const hits = clipboardAccessesIn(parseSource(path, text));
    return hits.map((line) => `${where}:${line}`);
  });
}

describe("one clipboard spelling", () => {
  it("is reached for nowhere but the hook that owns it", () => {
    expect(sitesAcrossTheTree()).toEqual([]);
  });

  it("finds the access however the caller spells it", () => {
    // Under-recognition is the one way this gate can break silently: a walk
    // that matched nothing would read a whole tree, report an empty list, and
    // pass. So the matcher is put in front of the shapes the eight sites
    // actually used — a direct call, an alias, a guard, the window-qualified
    // form — and asked to see every one.
    const planted = parseSource(
      "planted.tsx",
      [
        "await navigator.clipboard.writeText(a);",
        "const writer = navigator.clipboard;",
        "if (!navigator.clipboard) return;",
        "window.navigator.clipboard.writeText(b).then(ok, no);",
      ].join("\n"),
    );

    expect(clipboardAccessesIn(planted)).toEqual([1, 2, 3, 4]);
  });

  it("lets through every shape the matcher can see", () => {
    // The prefilter is the one place this gate could start reading a SMALLER
    // tree and still report PASS, so it is asked about the same four shapes
    // rather than trusted for being obvious. A fifth spelling that the matcher
    // catches but the filter drops would be invisible in every other assertion
    // here.
    for (const spelling of [
      "await navigator.clipboard.writeText(a);",
      "const writer = navigator.clipboard;",
      "if (!navigator.clipboard) return;",
      "window.navigator.clipboard.writeText(b).then(ok, no);",
    ]) {
      expect(mayReachForTheClipboard(spelling)).toBe(true);
    }
  });

  it("reads a corpus that is actually there", () => {
    // A census that can fail short has already failed: if the walk returned
    // nothing — a moved directory, a changed extension — the first assertion
    // would pass on an empty tree and this gate would be decoration.
    expect(filesUnder(join(frontendRoot, "src")).length).toBeGreaterThan(100);
    expect(
      readFileSync(
        join(frontendRoot, "src/design-system/clipboardcopy.tsx"),
        "utf8",
      ),
    ).toContain("navigator.clipboard");
  });
});
