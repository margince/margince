// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { sourceFileAt } from "../../scripts/lib/source-tree";
import { stable } from "../../src/format/collate";
import { type Area, plantedFindings, srcDir } from "../tree";
import {
  COMPONENT,
  findingsIn,
  NODE_BUILDER,
  rule,
} from "./heading-spelling.census";

// The size-to-element table exists twice: `heading.tsx` for the app and
// `bridge.ts` for the standalone views, which have no React. Read from the
// syntax tree so this test does not pull the views' tier in.
const TABLES: Readonly<Record<string, string>> = {
  [COMPONENT]: "DEFAULT_ELEMENT",
  [NODE_BUILDER]: "HEADING_ELEMENT",
};

/** The `{ key: "value" }` a module declares under `name`, as a plain map. */
function tableIn(path: string, name: string): Record<string, string> {
  const source = sourceFileAt(path);
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
      `${path} declares no ${name}; renaming it empties this mirror check`,
    );
  }
  return entries;
}

describe("heading spelling", () => {
  it("sees a raw heading in every shape it claims to", () => {
    expect(
      findingsIn("planted.tsx", "const a = () => <h2>Northwind</h2>;"),
    ).toHaveLength(1);
    // The opening tag over several lines, as a formatter writes it.
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
    // Prose about a heading, and a component whose name starts with an h.
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

  it("reports a planted raw heading through its own corpus", () => {
    const area: Area = "src";
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.tsx"),
          area,
          text: "export const A = () => <h2>Northwind</h2>;",
        },
        {
          path: COMPONENT,
          area,
          text: "export const H = () => <h2>Northwind</h2>;",
        },
      ]),
    ).toEqual([
      "src/features/planted.tsx:1 — <h2> is a raw heading; write <Heading size=… /> from design-system/heading",
    ]);
  });

  // A key on one side only, or one both carry with different elements, is the
  // defect whichever side it is on.
  it("keeps the app's size table and the view builder's identical", () => {
    const [component, builder] = [COMPONENT, NODE_BUILDER].map((path) =>
      tableIn(path, TABLES[path]),
    );
    const keys = [
      ...new Set([...Object.keys(component), ...Object.keys(builder)]),
    ].sort(stable);
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
});
