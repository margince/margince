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
// Seven guarded the API object and one did not; of the seven, one guarded it
// and then returned in silence. The notice was a `Callout` on five, a tinted `span` on one, a
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
// walk matches the ACCESS itself rather than a call to `writeText`.

const frontendRoot = resolve(__dirname, "..", "..");
const repoRoot = resolve(frontendRoot, "..");
const extensionsDir = join(repoRoot, "extensions");

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
 * matcher below fires on a property the source NAMES — an identifier after a
 * dot, or a static string in brackets — and either spelling has to appear
 * literally in the text for the parser to build that node. So every file the
 * matcher could flag contains this substring, and the filter is strictly WIDER
 * than the thing it stands in front of.
 *
 * It is here because it was measured, not because it looked faster: parsing all
 * 2251 files costs ~1.9s on a quiet machine and timed out past 10s on a loaded
 * four-way CI shard, while 28 files carry the word at all — ~320ms. A gate that
 * reds the build on how busy the runner is teaches everyone to re-run it.
 */
function mayReachForTheClipboard(text: string): boolean {
  return text.includes("clipboard");
}

/**
 * The property this node reads, where the source itself says which one: the
 * identifier after a dot, or a static string in brackets. `navigator.clipboard`
 * and `navigator["clipboard"]` are one reach written two ways, and a matcher
 * that knew only the first would pass a file silently — under-recognition being
 * the failure this gate has no assertion to notice.
 *
 * A COMPUTED key (`navigator[whicheverApi]`) yields null, and that is the
 * honest answer rather than a miss dressed up as a match: nothing in the text
 * says what the key holds, so no parser can decide it. The gate claims the
 * spellings it can prove and none it cannot.
 */
function staticPropertyName(node: ts.Node): string | null {
  if (ts.isPropertyAccessExpression(node)) {
    return node.name.text;
  }
  if (
    ts.isElementAccessExpression(node) &&
    ts.isStringLiteralLike(node.argumentExpression)
  ) {
    return node.argumentExpression.text;
  }
  return null;
}

/**
 * Whether a binding pattern takes `clipboard` off the navigator.
 *
 * `const { clipboard } = navigator` is the same reach as `navigator.clipboard`
 * written the other way round, and the alias this gate's header names as the
 * offence it exists to catch. A rename (`{ clipboard: writer }`) still binds
 * the same property, so the PROPERTY is what is read, never the local name.
 */
function destructuresTheClipboard(node: ts.Node): boolean {
  if (
    !ts.isVariableDeclaration(node) ||
    node.initializer === undefined ||
    !ts.isObjectBindingPattern(node.name)
  ) {
    return false;
  }
  if (!isTheNavigator(node.initializer)) {
    return false;
  }
  return node.name.elements.some(
    (element) =>
      (element.propertyName ?? element.name).getText() === "clipboard",
  );
}

/** Whether `expression` names the navigator, however it was reached. */
function isTheNavigator(expression: ts.Expression): boolean {
  // Bare, `window.navigator`, `globalThis["navigator"]`: what matters is the
  // last name in the chain, not the route taken to it.
  if (ts.isIdentifier(expression)) {
    return expression.text === "navigator";
  }
  return staticPropertyName(expression) === "navigator";
}

/** Every line in `source` that reaches for `navigator.clipboard`. */
function clipboardAccessesIn(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node) => {
    if (
      ((ts.isPropertyAccessExpression(node) ||
        ts.isElementAccessExpression(node)) &&
        staticPropertyName(node) === "clipboard" &&
        isTheNavigator(node.expression)) ||
      destructuresTheClipboard(node)
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

/**
 * Every file the gate judges: the app, and the extension frontends beside it.
 *
 * Separate from the walk so the census can be COUNTED before it is read — a
 * gate whose corpus quietly shrank reports PASS, and there is no failing
 * assertion to notice it.
 */
function filesInTheCensus(): string[] {
  return [
    ...filesUnder(join(frontendRoot, "src")),
    ...extensionFrontendFiles(extensionsDir),
  ];
}

function sitesIn(files: readonly string[]): string[] {
  return files.flatMap((path) => {
    const where = relative(repoRoot, path);
    if (OWNERS.has(where)) return [];
    const text = readFileSync(path, "utf8");
    if (!mayReachForTheClipboard(text)) return [];
    const hits = clipboardAccessesIn(parseSource(path, text));
    return hits.map((line) => `${where}:${line}`);
  });
}

// One spelling per line: the four shapes the eight hand-rolled sites used, and
// the bracketed forms of the same reach. Both assertions below read this list,
// so a spelling can be added in one place and neither the matcher nor the
// prefilter can quietly stop covering it.
const PLANTED_SPELLINGS = [
  "await navigator.clipboard.writeText(a);",
  "const writer = navigator.clipboard;",
  "if (!navigator.clipboard) return;",
  "window.navigator.clipboard.writeText(b).then(ok, no);",
  'await navigator["clipboard"].writeText(c);',
  'const bracketed = window.navigator["clipboard"];',
  'window["navigator"].clipboard.writeText(d).then(ok, no);',
  "const { clipboard } = navigator;",
  "const { clipboard: writer } = window.navigator;",
] as const;

describe("one clipboard spelling", () => {
  // A generous timeout, stated rather than defaulted: this reads and parses the
  // whole tree, which is not a unit test's shape of work. The prefilter keeps
  // the PARSE near a second; the 2251 reads in front of it do not shrink, and a
  // cold page cache took this past the 10s default twice.
  it("is reached for nowhere but the hook that owns it", {
    timeout: 60_000,
  }, () => {
    const files = filesInTheCensus();

    // A census that judged nothing certifies nothing. The count floor cannot
    // notice the extension half's loss on its own — a handful of files against
    // a floor of 100 — so that half is asserted by name, the mutant having
    // already survived once in this tree.
    expect(files.length).toBeGreaterThan(100);
    expect(
      files.some((file) => file.startsWith(`${extensionsDir}/`)),
      "the census covered frontend/src but no extension frontend layer",
    ).toBe(true);

    expect(sitesIn(files)).toEqual([]);
  });

  it("finds the access however the caller spells it", () => {
    // Under-recognition is the one way this gate can break silently: a walk
    // that matched nothing would read a whole tree, report an empty list, and
    // pass. So the matcher is put in front of every planted spelling and asked
    // for one hit on every line — a spelling it stops seeing is a line number
    // that goes missing rather than a difference nothing reports.
    const planted = parseSource("planted.tsx", PLANTED_SPELLINGS.join("\n"));

    expect(clipboardAccessesIn(planted)).toEqual(
      PLANTED_SPELLINGS.map((_, index) => index + 1),
    );
  });

  it("lets through every shape the matcher can see", () => {
    // The prefilter is the one place this gate could start reading a SMALLER
    // tree and still report PASS, so it is asked about the same spellings
    // rather than trusted for being obvious. One the matcher catches and the
    // filter drops would be invisible in every other assertion here.
    for (const spelling of PLANTED_SPELLINGS) {
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
