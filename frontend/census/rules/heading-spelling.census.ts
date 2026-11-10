// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A heading has one spelling, and it is `Heading`.
//
// `<h2>` decides two things with one keystroke: the type and the place in the
// outline. `Heading` splits them into `size` and `as`, reads the type from the
// `--fontHeading*` tokens, and brings no margin.
//
// Two arms, one per way a heading element reaches the DOM. One is a JSX
// `h1`-`h6` anywhere, tests and stories included. The other is a call that
// builds one from a string tag (`createElement("h2", …)`, the views'
// `el("h2", …)`).
//
// Two files may name the element: `heading.tsx` is the component, and
// `mcp-apps/bridge.ts` is the node builder the standalone views draw through.

import { join } from "node:path";
import ts from "typescript";
import { parseSource } from "../../scripts/lib/source-tree";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

export const COMPONENT = join(srcDir, "design-system", "heading.tsx");
export const NODE_BUILDER = join(srcDir, "mcp-apps", "bridge.ts");

// A floor under the walk: the tree carries several hundred modules.
const CORPUS_FLOOR = 100;

const HEADING_TAG = /^h[1-6]$/;

// The calls that build a node from a string tag. A query is not a build:
// `container.querySelector("h2")` is a test asking what got rendered.
const BUILDERS = new Set(["createElement", "el"]);

function tagNameOf(node: ts.JsxOpeningLikeElement): string {
  return node.tagName.getText(node.getSourceFile());
}

/** The identifier a call is made through: `el`, `createElement`, `React.createElement`. */
function calleeName(expression: ts.Expression): string {
  if (ts.isPropertyAccessExpression(expression)) return expression.name.text;
  return ts.isIdentifier(expression) ? expression.text : "";
}

/** Every raw heading in one module; `where` is how a finding names the file. */
export function findingsIn(path: string, text: string, where = path): string[] {
  const source = parseSource(path, text);
  const findings: string[] = [];
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

  const visit = (node: ts.Node) => {
    // The opening tag carries the name however many lines it spans.
    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      HEADING_TAG.test(tagNameOf(node))
    ) {
      findings.push(
        `${where}:${at(node)} — <${tagNameOf(node)}> is a raw heading; write <Heading size=… /> from design-system/heading`,
      );
    }
    if (ts.isCallExpression(node)) {
      const [first] = node.arguments;
      if (
        first !== undefined &&
        ts.isStringLiteralLike(first) &&
        HEADING_TAG.test(first.text) &&
        BUILDERS.has(calleeName(node.expression))
      ) {
        findings.push(
          `${where}:${at(node)} — ${calleeName(node.expression)}("${first.text}", …) builds a raw heading; write <Heading size=… /> from design-system/heading`,
        );
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return findings;
}

// The core's `.ts` and `.tsx`, and every module of each extension layer: a
// unit's screen is shipped UI in the same bundle.
const isCorpus = (file: CensusFile) =>
  ((file.area === "src" && /\.tsx?$/.test(file.path)) ||
    (file.area === "extensions" && file.source !== undefined)) &&
  file.path !== COMPONENT &&
  file.path !== NODE_BUILDER;

export const rule: CensusRule = {
  name: "heading-spelling",
  corpus: (tree) => tree.files.filter(isCorpus),
  // An exemption that outlives its file stops matching anything, unseen.
  floors: (corpus, tree) => [
    ...[COMPONENT, NODE_BUILDER]
      .filter((allowed) => tree.at(allowed) === undefined)
      .map((allowed) => `${allowed} is exempt but absent`),
    ...(corpus.length >= CORPUS_FLOOR
      ? []
      : [`fewer than ${CORPUS_FLOOR} modules were read`]),
    ...(corpus.some((file) => file.area === "extensions")
      ? []
      : ["the census reached no extension frontend layer"]),
  ],
  visit: (file) => findingsIn(file.path, file.text, file.rel),
  advice:
    "A heading is <Heading size=… /> from design-system/heading, never a raw h1-h6.",
};
