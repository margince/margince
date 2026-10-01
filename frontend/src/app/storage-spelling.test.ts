// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, relative, resolve } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  parseSource,
} from "../../scripts/lib/source-tree";

// Web Storage has ONE spelling, `app/storage.ts`: a key stored anywhere else is
// missing from its registry, so a sign-out cannot forget it.

// Matched on the parsed name, so a comment never counts and an alias does; a
// store reached by a run-time key (`window[area]`) is invisible to any parser.

const frontendRoot = resolve(__dirname, "..", "..");
const repoRoot = resolve(frontendRoot, "..");
const extensionsDir = join(repoRoot, "extensions");

const OWNER = "frontend/src/app/storage.ts";

const STORAGE_GLOBALS: ReadonlySet<string> = new Set([
  "localStorage",
  "sessionStorage",
]);

// A test or a story seeds storage as a fixture; neither ships.
const FIXTURE = /\.(test|stories)\.[cm]?[jt]sx?$/;

// Measured: ~450ms to parse all ~1050 files, ~13ms for the ~10 naming one.
function mayReachForStorage(text: string): boolean {
  return text.includes("localStorage") || text.includes("sessionStorage");
}

function storageReachesIn(source: ts.SourceFile): number[] {
  const lines = new Set<number>();
  const visit = (node: ts.Node) => {
    if (
      (ts.isIdentifier(node) || ts.isStringLiteralLike(node)) &&
      STORAGE_GLOBALS.has(node.text)
    ) {
      lines.add(source.getLineAndCharacterOfPosition(node.getStart()).line + 1);
    }
    ts.forEachChild(node, visit);
  };
  ts.forEachChild(source, visit);
  return [...lines];
}

function filesInTheCensus(): string[] {
  return [
    ...filesUnder(join(frontendRoot, "src")),
    ...extensionFrontendFiles(extensionsDir),
  ].filter((path) => !FIXTURE.test(path));
}

function sitesIn(files: readonly string[]): string[] {
  return files.flatMap((path) => {
    const where = relative(repoRoot, path);
    if (where === OWNER) return [];
    const text = readFileSync(path, "utf8");
    if (!mayReachForStorage(text)) return [];
    return storageReachesIn(parseSource(path, text)).map(
      (line) => `${where}:${line}`,
    );
  });
}

// One reach per line, in every shape the tree has used or a rename would take.
const PLANTED_SPELLINGS = [
  "window.localStorage.getItem(key);",
  "globalThis.sessionStorage?.setItem(key, value);",
  "localStorage.removeItem(key);",
  "const store = sessionStorage;",
  'const bracketed = window["localStorage"];',
  "const { localStorage } = window;",
  "const { sessionStorage: store } = globalThis;",
  'const { "localStorage": quoted } = window;',
  'if ("sessionStorage" in window) run();',
  "type Kept = typeof localStorage;",
] as const;

// Mentions that are not a reach: a comment, and a name that only starts alike.
const PLANTED_INNOCENTS = [
  "// localStorage in a comment",
  "/** sessionStorage in a doc comment */",
  "const localStorageish = readStored(STORAGE_KEYS.theme);",
] as const;

describe("one Web Storage spelling", () => {
  it("is reached for nowhere but app/storage.ts", {
    timeout: 60_000,
  }, () => {
    const files = filesInTheCensus();

    // A census that judged nothing certifies nothing.
    expect(files.length).toBeGreaterThan(500);
    expect(
      files.some((file) => file.startsWith(`${extensionsDir}/`)),
      "the census covered frontend/src but no extension frontend layer",
    ).toBe(true);

    const sites = sitesIn(files);
    expect(sites, `\n${sites.join("\n")}\n`).toEqual([]);
  });

  it("finds the reach however the caller spells it", () => {
    const planted = parseSource(
      "planted.tsx",
      [...PLANTED_SPELLINGS, ...PLANTED_INNOCENTS].join("\n"),
    );

    expect(storageReachesIn(planted)).toEqual(
      PLANTED_SPELLINGS.map((_, index) => index + 1),
    );
  });

  it("fails a planted file, and spares only the owner and fixtures", () => {
    const dir = mkdtempSync(join(tmpdir(), "storage-spelling-"));
    try {
      const planted = join(dir, "planted.tsx");
      writeFileSync(planted, `// a comment\n${PLANTED_SPELLINGS[0]}\n`);

      expect(sitesIn([planted])).toEqual([`${relative(repoRoot, planted)}:2`]);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
    expect(sitesIn([join(repoRoot, OWNER)])).toEqual([]);
    expect(FIXTURE.test("shell.test.tsx")).toBe(true);
    expect(FIXTURE.test("buyerroom.stories.tsx")).toBe(true);
    expect(FIXTURE.test("buyerroomsession.ts")).toBe(false);
  });

  it("lets through every shape the matcher can see", () => {
    for (const spelling of PLANTED_SPELLINGS) {
      expect(mayReachForStorage(spelling)).toBe(true);
    }
  });

  it("reads an owner that actually reaches for storage", () => {
    const owner = join(repoRoot, OWNER);
    const reaches = storageReachesIn(
      parseSource(owner, readFileSync(owner, "utf8")),
    );

    expect(reaches.length).toBeGreaterThan(0);
  });
});
