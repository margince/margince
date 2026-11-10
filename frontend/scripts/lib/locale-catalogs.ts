// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

// Every shipped catalog of a locale, core and extension: a gate that names one
// file reads a smaller tree and still passes.

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..", "..");

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

export function readJsonCatalog(path: string): Catalog {
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

// Listed by git, so a unit catalog in any layer counts.
function listedUnitCatalogs(locale: string): string[] {
  const listed = execFileSync(
    "git",
    [
      "ls-files",
      "-z",
      "--cached",
      "--others",
      "--exclude-standard",
      "extensions",
    ],
    { cwd: repoRoot, encoding: "utf8" },
  );
  const catalog = new RegExp(`(?:^|/)frontend/(?:.+/)?${locale}\\.json$`);
  return listed
    .split("\0")
    .filter((path) => catalog.test(path) && !path.includes("/node_modules/"));
}

/** Each unit's `<locale>.json`, with the en.json beside it. */
export function listedUnitCopy(locale: string): LocaleCopy[] {
  return listedUnitCatalogs(locale)
    .sort()
    .map((path) => {
      const full = join(repoRoot, path);
      return {
        source: path,
        catalog: readJsonCatalog(full),
        english: readJsonCatalog(join(dirname(full), "en.json")),
      };
    });
}

/**
 * The catalog a parsed module declares as the object literal `name`. A value
 * that is not a quoted key and a string throws, so no entry goes unread.
 */
export function catalogDeclaredIn(
  source: ts.SourceFile,
  name: string,
): Catalog {
  const catalog: Record<string, string> = {};
  let found = false;
  const visit = (node: ts.Node): void => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === name &&
      node.initializer &&
      ts.isObjectLiteralExpression(node.initializer)
    ) {
      found = true;
      for (const property of node.initializer.properties) {
        if (
          !ts.isPropertyAssignment(property) ||
          !ts.isStringLiteral(property.name) ||
          !ts.isStringLiteralLike(property.initializer)
        ) {
          throw new Error(
            `${source.fileName}: a ${name} entry is not a quoted key and a string`,
          );
        }
        catalog[property.name.text] = property.initializer.text;
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  if (!found) {
    throw new Error(
      `${source.fileName} no longer declares ${name} as an object`,
    );
  }
  return catalog;
}

export function catalogEntries(catalogs: readonly LocaleCopy[]): LocaleEntry[] {
  return catalogs.flatMap(({ source, catalog, english }) =>
    Object.entries(catalog).map(
      ([key, value]) => [source, key, value, english[key] ?? ""] as const,
    ),
  );
}
