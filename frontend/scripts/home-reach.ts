// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Whether a change can reach Home's first load or the browser check that
// measures it, so the per-PR Home check runs only when it can
// (scripts/home-budget-scope.sh asks). Both come from the import graph, never
// from a list of paths.
//
//   node scripts/home-reach.ts <path from the repository root>...
//
// Prints the import path to the first changed file Home loads and exits 0, or
// says none and exits 1.

import { existsSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { importPathTo } from "./lib/import-reach.ts";
import { moduleSpecifiers, sourceFileAt } from "./lib/source-tree.ts";

const frontendDir = join(dirname(fileURLToPath(import.meta.url)), "..");
const repoDir = resolve(frontendDir, "..");

// The shell, as far as it loads before any screen: its lazy screens are
// `import()` edges, which a static walk does not follow.
export const SHELL_ENTRY = join(frontendDir, "src/main.tsx");
// The Home screen with everything it can load, lazy parts included.
export const HOME_SCREEN = join(frontendDir, "src/features/brief/brief.tsx");
// The browser check, with every helper and judge it imports.
export const BENCH_SPEC = join(frontendDir, "e2e/bench-home.spec.ts");

const CSS_IMPORT = /@import\s+(?:url\(\s*)?["']([^"']+)["']/g;

// A stylesheet is no module a walk parses: it is reached through the module
// that imports it, and through each relative `@import` it makes in turn.
function stylesheetReaches(
  file: string,
  changed: ReadonlySet<string>,
  seen: Set<string>,
): boolean {
  if (changed.has(file)) return true;
  if (seen.has(file) || !existsSync(file)) return false;
  seen.add(file);
  return [...readFileSync(file, "utf8").matchAll(CSS_IMPORT)].some(
    ([, specifier]) =>
      specifier.startsWith(".") &&
      stylesheetReaches(resolve(dirname(file), specifier), changed, seen),
  );
}

function importsChanged(file: string, changed: ReadonlySet<string>): boolean {
  if (changed.has(file)) return true;
  return moduleSpecifiers(sourceFileAt(file), "all").some(
    (specifier) =>
      specifier.endsWith(".css") &&
      stylesheetReaches(resolve(dirname(file), specifier), changed, new Set()),
  );
}

/** The import path from Home's first load to a changed file, or null. */
export function homeReach(changedFromRepo: readonly string[]): string[] | null {
  for (const entry of [SHELL_ENTRY, HOME_SCREEN, BENCH_SPEC]) {
    if (!existsSync(entry)) {
      throw new Error(`${entry} is gone: point home-reach at Home's entry`);
    }
  }
  const changed = new Set(changedFromRepo.map((path) => join(repoDir, path)));
  const reaches = (file: string) => importsChanged(file, changed);
  return (
    importPathTo(SHELL_ENTRY, reaches, "static") ??
    importPathTo([HOME_SCREEN, BENCH_SPEC], reaches, "all")
  );
}

function main(paths: readonly string[]) {
  const path = homeReach(paths);
  if (path === null) {
    process.stdout.write(
      "home-reach: neither Home's first load nor its check imports a changed file\n",
    );
    process.exit(1);
  }
  const shown = path.map((file) => file.slice(repoDir.length + 1));
  process.stdout.write(`home-reach: ${shown.join(" -> ")}\n`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2));
}
