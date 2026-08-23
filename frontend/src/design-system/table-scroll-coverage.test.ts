// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

// Fitness function for a table that runs past its box with no way to reach the
// rest of it.
//
// A settings column is 654px and a coverage matrix is one column per
// colleague, so a table overrunning its container is not an edge case, it is
// Tuesday. `TableScroll` (atoms.tsx) is the one box that handles it: it takes
// a tab stop and announces itself as a NAMED region, and only while it is
// actually holding something past its right edge — so a keyboard reader can
// get to the columns they cannot see, and is not given a tab stop in front of
// every table in the product for the few that overflow.
//
// A hand-written `overflow-x: auto` gives the mouse everything and the
// keyboard nothing. That is the whole defect, and it is silent: the page looks
// right, and the reader who cannot reach the columns has no way to report what
// they did not know was there. TableScroll's own docblock says four screens
// had each written the wrapper by hand; when this gate was written there were
// two more — `coverageexplorer` had no wrapper at all, and `importmapping` had
// `.import__scroll`, which was `overflow-x: auto` and nothing else.
//
// The subject is DERIVED: every `<table>` JSX element under src/, found by
// parsing. A new screen is enrolled the moment it draws one.
//
// WHAT THIS DOES NOT CATCH, deliberately: a wrapper element that is not a
// JSX ancestor in the same file — a screen that renders `<table>` into a
// `children` prop the wrapper supplies from another file. Nothing here does
// that, and following it needs the type checker.

const dsDir = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(dsDir, "..");

// The two components that ARE the box, so their own `<table>` is inside it by
// construction. Named files rather than a pattern: a pattern is how a third
// implementation joins the exemption without anybody deciding it should.
const THE_BOX_ITSELF = [
  // DataTable renders into a TableScroll two lines above its `<table>`.
  "design-system/atoms.tsx",
  // ListTable's `.lt-scroll` shares TableScroll's `useScrollRegion` hook, so it
  // takes the same tab stop and the same announced name. That is the sanctioned
  // second spelling, and it is sanctioned because it reuses the behaviour
  // rather than re-deriving it.
  "design-system/listtable.tsx",
];

/** The names of JSX elements enclosing `node`, innermost first. */
function ancestorElements(node: ts.Node): string[] {
  const names: string[] = [];
  for (let at = node.parent; at !== undefined; at = at.parent) {
    if (ts.isJsxElement(at)) {
      const tag = at.openingElement.tagName;
      names.push(tag.getText());
    }
  }
  return names;
}

/** `<file>:<line>` for every `<table>` in one file with no TableScroll above it. */
function unwrappedIn(fileName: string, text: string): string[] {
  const source = ts.createSourceFile(
    fileName,
    text,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TSX,
  );
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    const opening = ts.isJsxElement(node)
      ? node.openingElement
      : ts.isJsxSelfClosingElement(node)
        ? node
        : undefined;
    if (opening !== undefined && opening.tagName.getText() === "table") {
      if (!ancestorElements(node).includes("TableScroll")) {
        const { line } = source.getLineAndCharacterOfPosition(node.getStart());
        found.push(`${fileName}:${line + 1}`);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    return entry.isDirectory()
      ? sourceFiles(path)
      : /\.tsx$/.test(entry.name) && !/\.(test|stories)\.tsx$/.test(entry.name)
        ? [path]
        : [];
  });
}

function unwrappedTables(): string[] {
  const files = sourceFiles(srcRoot)
    .map((file) => [relative(srcRoot, file), file] as const)
    .filter(([name]) => !THE_BOX_ITSELF.includes(name))
    .filter(([, file]) => readFileSync(file, "utf8").includes("<table"));
  // A sweep that found no tables and a tree with none look identical, and both
  // look green.
  expect(files.length).toBeGreaterThan(0);
  return files
    .flatMap(([name, file]) => unwrappedIn(name, readFileSync(file, "utf8")))
    .sort();
}

const PARSE_MS = 60_000;

describe("a hand-drawn table", () => {
  it(
    "scrolls inside a box a keyboard reader can reach",
    () => {
      const found = unwrappedTables();
      expect(found, `\n${found.join("\n")}\n`).toEqual([]);
    },
    PARSE_MS,
  );

  const planted: ReadonlyArray<readonly [string, string, number]> = [
    ["a bare table", "const a = <table><tbody /></table>;", 1],
    [
      "a table under a hand-written scroll div, which is the defect",
      'const a = <div className="import__scroll"><table><tbody /></table></div>;',
      1,
    ],
    [
      "two bare tables, which a per-file flag would count once",
      "const a = <><table><tbody /></table><table><tbody /></table></>;",
      2,
    ],
    // What must stay invisible.
    [
      "a table inside TableScroll",
      'const a = <TableScroll label="x"><table><tbody /></table></TableScroll>;',
      0,
    ],
    [
      "a table several levels down inside TableScroll",
      'const a = <TableScroll label="x"><div><section><table><tbody /></table></section></div></TableScroll>;',
      0,
    ],
    [
      "a TableScroll that wraps something else entirely",
      'const a = <><TableScroll label="x"><p /></TableScroll><DataTable rows={r} /></>;',
      0,
    ],
  ];
  for (const [name, source, expected] of planted) {
    it(`sees ${name}`, () => {
      expect(unwrappedIn("planted.tsx", source)).toHaveLength(expected);
    });
  }
});
