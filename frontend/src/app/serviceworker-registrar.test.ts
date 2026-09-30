// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  parseSource,
  sourceFileAt,
} from "../../scripts/lib/source-tree";

// One registrar: a second could install a worker that pins browsers to a build.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

// Every module the product ships, in every script dialect, core and extension tier.
const shipped = [
  ...filesUnder(join(frontendRoot, "src")),
  ...extensionFrontendFiles(join(frontendRoot, "..", "extensions")),
].filter((file) => !/\.(test|stories)\.[cm]?[jt]sx?$/.test(file));

/** Whether a module names the worker container, in code rather than in a comment. */
function namesServiceWorker(source: ts.SourceFile): boolean {
  const visit = (node: ts.Node): boolean =>
    ((ts.isIdentifier(node) || ts.isStringLiteralLike(node)) &&
      node.text === "serviceWorker") ||
    (ts.forEachChild(node, visit) ?? false);
  return visit(source);
}

// It parses the whole tree, so it takes the conformance suite's scan budget, for its reason.
describe("the service worker's registrar", { timeout: 60_000 }, () => {
  it("is app/pwa.ts, and no other shipped module reaches for navigator.serviceWorker", () => {
    const named = shipped
      .filter((file) => namesServiceWorker(sourceFileAt(file)))
      .map((file) => relative(frontendRoot, file));
    expect(named).toEqual(["src/app/pwa.ts"]);
  });

  it("reads the plain scripts the tree ships, the worker's own source among them", () => {
    expect(shipped).toContain(
      join(frontendRoot, "src/offline/serviceworker.js"),
    );
  });

  it.each([
    ["aliased.js", "const container = navigator.serviceWorker;"],
    ["destructured.mjs", "const { serviceWorker: sw } = navigator;"],
    ["indexed.jsx", 'navigator["serviceWorker"].register("/x.js");'],
    ["templated.ts", "navigator[`serviceWorker`].register('/x.js');"],
  ])("sees the container reached for in %s", (file, text) => {
    expect(namesServiceWorker(parseSource(file, text))).toBe(true);
  });

  it("does not read a comment as a registration", () => {
    const text = "// navigator.serviceWorker.register('/x.js');\nexport {};";
    expect(namesServiceWorker(parseSource("comment.ts", text))).toBe(false);
  });
});
