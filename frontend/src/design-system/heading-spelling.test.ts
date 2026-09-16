// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesMatching, parseSource } from "../../scripts/lib/source-tree";

// A heading has one spelling, and it is `Heading`.
//
// `<h2>` asks a caller to decide two things with one keystroke — the type and
// the place in the document outline — and the tree answered them inconsistently
// for as long as it could: a modal title that was an `<h2>` on one surface and
// an `<h3>` on the next, both drawn at whatever size the UA or a nearby class
// happened to give them. `Heading` splits the two into `size` and `as`, reads
// the type from the `--fontHeading*` tokens, and brings no margin, so the
// passage around it keeps its own rhythm. None of that helps while a second
// spelling is still available, which is what this gate removes.
//
// Two arms, one per way a heading element reaches the DOM:
//
//   markup — a JSX element named `h1`–`h6` anywhere under `src/`, tests and
//            stories included. A test that hand-rolls an `<h2>` to satisfy a
//            dialog's `aria-labelledby` is the next author's example of how
//            headings are written here, and examples are how a retired spelling
//            comes back.
//   builder — a call whose first argument is the string `"h1"`–`"h6"`:
//            `createElement("h2", …)` and the `el("h2", …)` node builder the
//            standalone views use. A string tag gets past a JSX walk entirely,
//            so a gate with only the first arm reports PASS over a tree that
//            builds its headings the other way.
//
// The exemptions are the two files that are ALLOWED to name the element,
// because something has to: `heading.tsx` is the component, and
// `mcp-apps/bridge.ts` is the single node builder the standalone views draw
// through. Both are asserted to exist, so an exemption cannot outlive its file.
//
// Under-recognition is the one way this must not break — a walk that reads a
// smaller tree reports the same word, PASS — so the corpus is derived from the
// tree, it fails closed when it comes back small or without the component it
// protects, and the detector carries a planted case of every shape it claims
// to see, the multi-line opening tag included.

const dsDir = dirname(fileURLToPath(import.meta.url));
const srcDir = join(dsDir, "..");
const frontendRoot = join(srcDir, "..");

// The component itself, and the one builder allowed to name the element.
const COMPONENT = join(dsDir, "heading.tsx");
const NODE_BUILDER = join(srcDir, "mcp-apps", "bridge.ts");

// The size→element table exists twice on purpose — `heading.tsx` for the app,
// `bridge.ts` for the standalone views, which have no React to import the
// component from — and a mirror nothing holds is two tables. The gate below
// fails in BOTH directions: a key added, removed or changed on either side.
const TABLES: Readonly<Record<string, string>> = {
  [COMPONENT]: "DEFAULT_ELEMENT",
  [NODE_BUILDER]: "HEADING_ELEMENT",
};

// A floor under the walk. The tree carries several hundred modules; a corpus
// that comes back near this number means the walk broke, not that the SPA shrank.
const CORPUS_FLOOR = 100;

const HEADING_TAG = /^h[1-6]$/;

// The two calls that BUILD a node from a string tag. Named rather than left
// open, because a query is not a build: `container.querySelector("h2")` is a
// test asking what got rendered, and a gate that read it as a heading would
// fire on the very assertions proving the migration worked.
const BUILDERS = new Set(["createElement", "el"]);

function tagNameOf(node: ts.JsxOpeningLikeElement): string {
  return node.tagName.getText(node.getSourceFile());
}

/**
 * The `{ key: "value" }` a module declares under `name`, as a plain map.
 *
 * Read from the syntax tree rather than imported, because the two tables sit in
 * different tiers: `bridge.ts` builds DOM nodes for a page with no React root,
 * and a gate that imported it would pull the tier boundary into the test.
 */
function tableIn(path: string, name: string): Record<string, string> {
  const source = parseSource(path, readFileSync(path, "utf8"));
  const entries: Record<string, string> = {};
  let found = false;
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === name &&
      node.initializer !== undefined &&
      ts.isObjectLiteralExpression(node.initializer)
    ) {
      found = true;
      for (const property of node.initializer.properties) {
        if (
          ts.isPropertyAssignment(property) &&
          ts.isStringLiteralLike(property.initializer)
        ) {
          entries[property.name.getText(source)] = property.initializer.text;
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  if (!found) {
    throw new Error(
      `${relative(frontendRoot, path)} declares no ${name} — this gate mirrors it, so renaming it silently empties the gate`,
    );
  }
  return entries;
}

/** The identifier a call is made through: `el`, `createElement`, `React.createElement`. */
function calleeName(expression: ts.Expression): string {
  if (ts.isPropertyAccessExpression(expression)) return expression.name.text;
  return ts.isIdentifier(expression) ? expression.text : "";
}

function findingsIn(path: string, text: string): string[] {
  const source = parseSource(path, text);
  const findings: string[] = [];
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;

  const visit = (node: ts.Node) => {
    // The opening tag carries the name whether or not the element closes on
    // the same line, so a `<h1\n  className={…}>` spread over four lines is one
    // node here and needs no special case.
    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      HEADING_TAG.test(tagNameOf(node))
    ) {
      findings.push(
        `${relative(frontendRoot, path)}:${at(node)} — <${tagNameOf(node)}> is a raw heading; write <Heading size=… /> from design-system/heading`,
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
          `${relative(frontendRoot, path)}:${at(node)} — ${calleeName(node.expression)}("${first.text}", …) builds a raw heading; write <Heading size=… /> from design-system/heading`,
        );
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return findings;
}

// The parse is bounded and deterministic, so it states a budget a loaded runner
// cannot exhaust rather than living on vitest's 5s default, which is sized for
// an async UI test.
const PARSE_MS = 60_000;

describe("heading spelling", () => {
  it("sees a raw heading in every shape it claims to", () => {
    expect(
      findingsIn("planted.tsx", "const a = () => <h2>Northwind</h2>;"),
    ).toHaveLength(1);
    // The opening tag broken over several lines: the shape a formatter produces
    // the moment a heading carries two attributes, and the one a line-oriented
    // grep reads past.
    expect(
      findingsIn(
        "planted.tsx",
        'const a = () => (\n  <h1\n    className="lt-title"\n    id="x"\n  >\n    Northwind\n  </h1>\n);',
      ),
    ).toHaveLength(1);
    expect(
      findingsIn("planted.tsx", 'const a = () => <h3 className="x" />;'),
    ).toHaveLength(1);
    expect(
      findingsIn("planted.ts", 'export const a = el("h2", { id: "x" });'),
    ).toHaveLength(1);
    expect(
      findingsIn(
        "planted.ts",
        'export const a = React.createElement("h4", null);',
      ),
    ).toHaveLength(1);
  });

  it("does not fire on a heading written the one way", () => {
    expect(
      findingsIn(
        "planted.tsx",
        'const a = () => <Heading size="large">Northwind</Heading>;',
      ),
    ).toEqual([]);
    // A comment and a string naming the element are prose about headings, not
    // a heading; so is a component whose name merely starts with an h.
    expect(
      findingsIn(
        "planted.tsx",
        '// <h2> is how this used to be written.\nconst a = () => <h2Like label="<h3>" />;',
      ),
    ).toEqual([]);
    expect(
      findingsIn("planted.ts", 'export const a = ariaLevel("h2".length);'),
    ).toEqual([]);
    // Asking the DOM what it rendered is not building one.
    expect(
      findingsIn("planted.tsx", 'const found = container.querySelector("h2");'),
    ).toEqual([]);
  });

  // Two tables, one rule. Stated as a set comparison rather than a subset in
  // either direction: a key present on one side only is the defect whichever
  // side it is on, and so is a key both carry with different elements.
  it("keeps the app's size table and the view builder's identical", () => {
    const [component, builder] = [COMPONENT, NODE_BUILDER].map((path) =>
      tableIn(path, TABLES[path]),
    );
    const keys = [
      ...new Set([...Object.keys(component), ...Object.keys(builder)]),
    ].sort();
    expect(keys.length).toBeGreaterThan(0);
    const differing = keys.filter((key) => component[key] !== builder[key]);
    expect(
      differing,
      differing
        .map(
          (key) =>
            `${key}: heading.tsx says ${component[key] ?? "nothing"}, bridge.ts says ${builder[key] ?? "nothing"}`,
        )
        .join("\n"),
    ).toEqual([]);
  });

  it(
    "finds no raw heading element anywhere under src/",
    () => {
      // An exemption that outlives its file is a hole nobody sees, because the
      // path simply stops matching anything the walk returns.
      for (const allowed of [COMPONENT, NODE_BUILDER]) {
        expect(existsSync(allowed), `${allowed} is exempt but absent`).toBe(
          true,
        );
      }
      const corpus = filesMatching(srcDir, /\.tsx?$/).filter(
        (path) => path !== COMPONENT && path !== NODE_BUILDER,
      );
      // Both floors, because a walk that returns nothing and a tree that holds
      // nothing look identical from here, and both look like success.
      expect(corpus.length).toBeGreaterThanOrEqual(CORPUS_FLOOR);
      expect(existsSync(COMPONENT)).toBe(true);

      const findings = corpus.flatMap((path) =>
        findingsIn(path, readFileSync(path, "utf8")),
      );
      expect(findings, `\n${findings.join("\n")}\n`).toEqual([]);
    },
    PARSE_MS,
  );
});
