// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { de } from "../../src/i18n/de";
import { extensionLayers, filesMatching } from "./source-tree";

// The German copy every German gate reads. The corpus is DERIVED from every
// German catalog a bundler ships, core and extension: a gate naming one file
// reads a smaller tree and still says PASS.

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..", "..");
const extensionsDir = resolve(repoRoot, "extensions");

type Catalog = Readonly<Record<string, string>>;
type GermanCopy = Readonly<{ source: string; catalog: Catalog }>;

// The letters a German word is made of. `\b` answers for none of the umlauts,
// so a boundary written with it would match inside a word that carries one.
export const LETTER = "A-Za-zÄÖÜäöüß";

function readJsonCatalog(path: string): Catalog {
  const parsed: unknown = JSON.parse(readFileSync(path, "utf8"));
  if (typeof parsed !== "object" || parsed === null) {
    throw new Error(`${path} is not a JSON object of message keys`);
  }
  const catalog: Record<string, string> = {};
  for (const [key, value] of Object.entries(parsed)) {
    if (typeof value !== "string") {
      throw new Error(`${path}: ${key} does not hold a string`);
    }
    catalog[key] = value;
  }
  return catalog;
}

// Every unit's German catalog, found the way a bundler finds one: a file named
// de.json inside a frontend layer, at whatever depth the unit put it.
function unitCopy(): GermanCopy[] {
  return extensionLayers(extensionsDir)
    .flatMap((layer) => filesMatching(layer, /^de\.json$/))
    .sort()
    .map((path) => ({
      source: relative(repoRoot, path),
      catalog: readJsonCatalog(path),
    }));
}

export const GERMAN: readonly GermanCopy[] = [
  { source: "frontend/src/i18n/de.ts", catalog: de },
  ...unitCopy(),
];

/** Every German value as `[source, key, value]`. */
export function entries(): Array<readonly [string, string, string]> {
  return GERMAN.flatMap(({ source, catalog }) =>
    Object.entries(catalog).map(
      ([key, value]) => [source, key, value] as const,
    ),
  );
}
