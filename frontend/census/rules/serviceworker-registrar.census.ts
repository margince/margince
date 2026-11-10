// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One registrar: a second could install a worker that pins browsers to a build.
// lib/pwa.ts is the only shipped module that names the worker container.

import { join } from "node:path";
import ts from "typescript";
import { type CensusRule, srcDir } from "../tree";

const registrar = join(srcDir, "lib", "pwa.ts");
const workerSource = join(srcDir, "offline", "serviceworker.js");

/** Whether a module names the worker container, in code rather than in a comment. */
export function namesServiceWorker(source: ts.SourceFile): boolean {
  const visit = (node: ts.Node): boolean =>
    ((ts.isIdentifier(node) || ts.isStringLiteralLike(node)) &&
      node.text === "serviceWorker") ||
    (ts.forEachChild(node, visit) ?? false);
  return visit(source);
}

export const rule: CensusRule = {
  name: "serviceworker-registrar",
  // Every module the product ships, in every script dialect, core and units.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.source !== undefined &&
        !/\.(test|stories)\.[cm]?[jt]sx?$/.test(file.path),
    ),
  floors: (corpus) => [
    ...(corpus.some((file) => file.path === workerSource)
      ? []
      : ["src/offline/serviceworker.js, a plain script, was not read"]),
    ...(corpus.some((file) => file.path === registrar)
      ? []
      : ["src/lib/pwa.ts was not read"]),
  ],
  visit: (file) => {
    const names = file.source ? namesServiceWorker(file.source) : false;
    if (file.path === registrar) {
      return names
        ? []
        : [`${file.rel} no longer names navigator.serviceWorker`];
    }
    return names ? [`${file.rel} names navigator.serviceWorker`] : [];
  },
  advice:
    "The service worker has one registrar, lib/pwa.ts. Reach the worker " +
    "through it rather than through navigator.serviceWorker.",
};
