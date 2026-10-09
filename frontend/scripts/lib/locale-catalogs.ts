// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { en } from "../../src/i18n/en";
import { extensionLayers, filesMatching } from "./source-tree";

// Derived from every shipped catalog of a locale, core and extension: a gate
// that names one file reads a smaller tree and still passes.

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..", "..");
const extensionsDir = resolve(repoRoot, "extensions");

type Catalog = Readonly<Record<string, string>>;
export type LocaleCopy = Readonly<{
  source: string;
  catalog: Catalog;
  english: Catalog;
}>;
export type LocaleEntry = readonly [
  source: string,
  key: string,
  value: string,
  english: string,
];

function readJsonCatalog(path: string): Catalog {
  const parsed: unknown = JSON.parse(readFileSync(path, "utf8"));
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
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

// Found the way a bundler finds one: a `<locale>.json` inside a frontend layer,
// at whatever depth the unit put it.
function unitCopy(locale: string): LocaleCopy[] {
  return extensionLayers(extensionsDir)
    .flatMap((layer) => filesMatching(layer, new RegExp(`^${locale}\\.json$`)))
    .sort()
    .map((path) => ({
      source: relative(repoRoot, path),
      catalog: readJsonCatalog(path),
      english: readJsonCatalog(join(dirname(path), "en.json")),
    }));
}

export function localeCatalogs(
  locale: string,
  core: Catalog,
): readonly LocaleCopy[] {
  return [
    { source: `frontend/src/i18n/${locale}.ts`, catalog: core, english: en },
    ...unitCopy(locale),
  ];
}

export function catalogEntries(catalogs: readonly LocaleCopy[]): LocaleEntry[] {
  return catalogs.flatMap(({ source, catalog, english }) =>
    Object.entries(catalog).map(
      ([key, value]) => [source, key, value, english[key] ?? ""] as const,
    ),
  );
}
