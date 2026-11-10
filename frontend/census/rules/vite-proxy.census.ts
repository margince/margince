// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every origin-relative address the product hands somebody answers on the
// app's own port.
//
// The dev server is the app's origin and proxies a fixed list of prefixes; the
// api is a port a reader never sees. A command built from `location.origin`
// would 404 there while the endpoint answers one port over.
//
// So the prefixes are derived from what the code builds, and a second
// connector publishing its own edge is covered the day it is written.

import { dirname, join, resolve } from "node:path";
import spaConfig from "../../vite.config";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

// A template literal that opens with an interpolation, `${anything}/x/…`, and
// the segment after it, which is what a proxy entry is keyed on. Anchored on
// the backtick, so `${origin}/v1/offers/${id}/pdf` is not read as /pdf.
const pathTemplate = /`\$\{[^}]*\}\/([a-z0-9._-]+)/g;

// The same shape, narrowed to a root interpolation that says origin.
const namedOrigin = /`\$\{[^}]*\borigin\b[^}]*\}\/([a-z0-9._-]+)/gi;

// The specifiers a module pulls from beside itself.
const relativeImport = /\bfrom\s+"(\.[^"]*)"/g;

/** The proxy keys, the one list of what the api owns on this origin. */
export const proxiedPrefixes = (): string[] =>
  Object.keys(spaConfig.server?.proxy ?? {});

/**
 * The files an address could be assembled across: the one reading
 * `location.origin`, and the modules beside it that it imports. The reader of
 * the origin and the builder of the path are often two files, so renaming a
 * parameter away from `origin` cannot shrink the census.
 */
function neighbourhood(file: CensusFile, tree: CensusTree): CensusFile[] {
  const out = [file];
  for (const match of file.text.matchAll(relativeImport)) {
    const base = resolve(dirname(file.path), match[1]);
    for (const candidate of [
      `${base}.ts`,
      `${base}.tsx`,
      join(base, "index.ts"),
    ]) {
      const near = tree.at(candidate);
      if (near) out.push(near);
    }
  }
  return out;
}

export type Addresses = Readonly<{
  built: Map<string, string>;
  byNeighbourhood: Set<string>;
}>;

/**
 * The prefixes the corpus builds addresses at, each with a file that builds it.
 * Two readings are unioned: a template rooted at something saying origin, and
 * every path template near a file that reads `location.origin`.
 */
export function builtAddresses(
  corpus: readonly CensusFile[],
  tree: CensusTree,
): Addresses {
  const built = new Map<string, string>();
  const byNeighbourhood = new Set<string>();
  for (const file of corpus) {
    for (const match of file.text.matchAll(namedOrigin)) {
      built.set(match[1], file.rel);
    }
    if (!/\blocation\.origin\b/.test(file.text)) continue;
    for (const near of neighbourhood(file, tree)) {
      for (const match of near.text.matchAll(pathTemplate)) {
        built.set(match[1], near.rel);
        byNeighbourhood.add(match[1]);
      }
    }
  }
  return { built, byNeighbourhood };
}

export const rule: CensusRule = {
  name: "vite-proxy",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        /\.tsx?$/.test(file.path) &&
        !/\.test\.tsx?$/.test(file.path),
    ),
  // A reading that found nothing would pass over an empty set. The second
  // reading must find something of its own, or the union is carried by the
  // first alone.
  floors: (corpus, tree) => {
    const { built, byNeighbourhood } = builtAddresses(corpus, tree);
    return [
      ...(proxiedPrefixes().length > 4
        ? []
        : ["vite.config.ts proxies 4 prefixes or fewer"]),
      ...(built.size > 0 ? [] : ["no address built from the origin was found"]),
      ...(byNeighbourhood.size > 0
        ? []
        : [
            "no address was found through a file reading location.origin; the reading that does not depend on an identifier named `origin` reaches nothing",
          ]),
    ];
  },
  across: (corpus, tree) => {
    const proxied = new Set(proxiedPrefixes());
    return [...builtAddresses(corpus, tree).built]
      .filter(([prefix]) => !proxied.has(`/${prefix}`))
      .map(
        ([prefix, file]) =>
          `${file} builds an address at /${prefix} from the app's own origin, which the dev server does not proxy`,
      );
  },
  advice:
    "The command such a file hands a reader 404s on the app's port while the " +
    "endpoint answers one port over. Add the prefix to the proxy list in " +
    "vite.config.ts.",
};
