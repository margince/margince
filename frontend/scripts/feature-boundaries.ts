// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two rules that keep each feature folder its own, held by a ratchet.
//
//  1. Only the shell (app/ and App) imports a feature. A feature imports no
//     other feature, and never the shell.
//  2. A file in components/, lib/ or format/ is shared, so it is never used by
//     one feature alone. Such a file belongs in that feature.
//
// Each rule fails what the other lets through. Without the second, the cheap
// way past the first is to move one feature's code into a shared folder.
// scripts/feature-boundaries-waivers.txt freezes the findings the tree had
// when the gate landed. It may only get shorter.
import { existsSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { reachedModules } from "./lib/bundle-reach.ts";
import {
  filesMatching,
  MODULE_FILE,
  moduleAt,
  moduleSpecifiers,
  sourceFileAt,
} from "./lib/source-tree.ts";

const frontendDir = join(dirname(fileURLToPath(import.meta.url)), "..");
export const SRC = join(frontendDir, "src");
export const LEDGER = join(
  frontendDir,
  "scripts",
  "feature-boundaries-waivers.txt",
);

/** Each source file and the files it imports, relative to src/ with `/`. */
export type ImportGraph = ReadonlyMap<string, readonly string[]>;

const SHELL = new Set(["app", "App"]);
// Test harnesses mount features, and a fork's own screens may use them. The
// waiver list is ours, and a fork must never have to edit it.
const MAY_IMPORT_A_FEATURE = new Set([...SHELL, "testing", "custom"]);
const SHARED = new Set(["components", "lib", "format"]);

// ownerOf names the folder a file belongs to: `features/<name>` inside a
// feature, `App` for the App.* files at the root, else the top-level folder.
export function ownerOf(path: string): string {
  const [top, next] = path.split("/");
  if (top === "features" && next !== undefined) return `features/${next}`;
  return /^App\./.test(top) ? "App" : top;
}

// reachFindings lists rule 1's findings as `reach <from> <to>` lines.
export function reachFindings(graph: ImportGraph): string[] {
  const found = new Set<string>();
  for (const [from, targets] of graph) {
    const own = ownerOf(from);
    const isFeature = own.startsWith("features/");
    if (!isFeature && MAY_IMPORT_A_FEATURE.has(own)) continue;
    for (const to of targets) {
      const other = ownerOf(to);
      if (other === own) continue;
      const entersFeature = other.startsWith("features/");
      if (entersFeature || (isFeature && SHELL.has(other))) {
        found.add(`reach ${from} ${to}`);
      }
    }
  }
  return [...found];
}

// ownedFindings lists rule 2's findings as `owned <path> <feature>` lines.
// Only a shipped importer counts as a user, and `shipped` answers that from
// the bundle's own entries. A file another shared file imports counts as
// shared, whoever imports that one.
export function ownedFindings(
  graph: ImportGraph,
  shipped: (path: string) => boolean,
): string[] {
  const users = new Map<string, Set<string>>();
  for (const [from, targets] of graph) {
    if (!shipped(from)) continue;
    for (const to of targets) {
      const owners = users.get(to) ?? new Set<string>();
      owners.add(ownerOf(from));
      users.set(to, owners);
    }
  }
  const found: string[] = [];
  for (const [path, owners] of users) {
    if (!SHARED.has(ownerOf(path))) continue;
    const [only] = owners;
    if (owners.size === 1 && only.startsWith("features/")) {
      found.push(`owned ${path} ${only}`);
    }
  }
  return found;
}

// shippedInTree answers rule 2's question for the real tree: a module counts
// when the bundle reaches it. A stylesheet ships with the module importing it.
export function shippedInTree(root = SRC): (path: string) => boolean {
  const reached = reachedModules();
  return (path) => path.endsWith(".css") || reached.has(join(root, path));
}

const CSS_IMPORT = /@import\s+(?:url\()?["']([^"']+)["']/g;
const MOCKS = new Set(["mock", "doMock", "importActual", "importMock"]);

// mockedPaths lists the modules a test swaps out with `vi.mock` and its kin. A
// test that mocks another feature's module depends on that module's path.
function mockedPaths(source: ts.SourceFile): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      ts.isIdentifier(node.expression.expression) &&
      node.expression.expression.text === "vi" &&
      MOCKS.has(node.expression.name.text) &&
      node.arguments.length > 0 &&
      ts.isStringLiteralLike(node.arguments[0])
    ) {
      found.push(node.arguments[0].text);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

function isFile(path: string): boolean {
  return existsSync(path) && statSync(path).isFile();
}

// importsOf resolves what one file imports, stylesheets and assets included:
// a feature that pulls in another feature's CSS depends on its class names.
function importsOf(file: string): string[] {
  let specifiers: string[];
  if (file.endsWith(".css")) {
    specifiers = [...readFileSync(file, "utf8").matchAll(CSS_IMPORT)].map(
      (match) => match[1],
    );
  } else {
    const source = sourceFileAt(file);
    specifiers = [...moduleSpecifiers(source, "all"), ...mockedPaths(source)];
  }
  return specifiers.flatMap((specifier) => {
    if (!specifier.startsWith(".")) return [];
    const path = join(dirname(file), specifier.replace(/\?.*$/, ""));
    const hit = moduleAt(path) ?? (isFile(path) ? path : null);
    return hit === null ? [] : [hit];
  });
}

const READ = new RegExp(`${MODULE_FILE.source}|\\.css$`);

// readGraph reads the import graph of every module and stylesheet under `root`.
export function readGraph(root = SRC): ImportGraph {
  const key = (path: string) => relative(root, path).split(sep).join("/");
  return new Map(
    filesMatching(root, READ).map((file) => [
      key(file),
      [...new Set(importsOf(file).map(key))],
    ]),
  );
}

// parseLedger reads the waiver lines, skipping comments and blank lines. A
// line twice would outlive its fix, since the fix deletes only one of them.
export function parseLedger(text: string): Set<string> {
  const ledger = new Set<string>();
  for (const [index, raw] of text.split("\n").entries()) {
    const line = raw.trim();
    if (line === "" || line.startsWith("#")) continue;
    if (!/^(reach|owned) \S+ \S+$/.test(line)) {
      throw new Error(
        `${LEDGER}:${index + 1}: expected "reach <from> <to>" or "owned <path> <feature>", got "${line}"`,
      );
    }
    if (ledger.has(line)) {
      throw new Error(`${LEDGER}:${index + 1}: "${line}" is listed twice`);
    }
    ledger.add(line);
  }
  return ledger;
}

// fixFor says how to fix a new finding, because an agent reads it and acts on
// it. Paths carry `src/`, the form scripts/move-modules.ts takes.
function fixFor(line: string, where: string): string {
  const [kind, path, other] = line.split(" ");
  const never = `Never add the line to ${where}.`;
  if (kind === "owned") {
    return (
      `src/${path} is shared code, but only ${other} uses it. Move it into ` +
      `src/${other}/ with node scripts/move-modules.ts. ${never}`
    );
  }
  return (
    `src/${path} imports src/${other}. Only app/ and App import a feature, and a ` +
    "feature imports no other feature and not the shell. Move the imported file: " +
    "to src/testing/ if it is a test, story, testkit or fixture helper; to " +
    "src/components/ if it draws and two or more features use it; to src/lib/ if " +
    "it draws nothing; into the importing feature if no other uses it. If it is " +
    "another feature's whole screen, let app/ pass it in. Move files with node " +
    `scripts/move-modules.ts. ${never}`
  );
}

// judge lists every way the findings and the ledger disagree.
export function judge(found: readonly string[], ledger: Set<string>): string[] {
  const where = relative(frontendDir, LEDGER);
  const failures = found
    .filter((line) => !ledger.has(line))
    .map((line) => fixFor(line, where));
  const current = new Set(found);
  for (const line of ledger) {
    if (!current.has(line)) {
      failures.push(`${line}: fixed. Delete its line from ${where}.`);
    }
  }
  return failures.sort();
}
