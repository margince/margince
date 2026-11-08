// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import { describe, expect, it } from "vitest";
import { mayHoldLiteral } from "./literal-text";
import { parseSource } from "./source-tree";

const PANE = /--pane(?![\w-])/;

function literalTexts(source: string): string[] {
  const texts: string[] = [];
  const walk = (node: ts.Node) => {
    if (ts.isStringLiteralLike(node) || ts.isTemplateLiteralToken(node)) {
      texts.push(node.text);
    }
    ts.forEachChild(node, walk);
  };
  walk(parseSource("probe.tsx", source));
  return texts;
}

describe("a source holding a literal", () => {
  it.each([
    ["a hex escape", String.raw`const a = "\x2d-pane";`],
    ["a unicode escape", String.raw`const a = "\u002d-pane";`],
    ["a braced unicode escape", String.raw`const a = "\u{2d}-pane";`],
    ["a legacy octal escape", String.raw`const a = "\055-pane";`],
    ["a control escape", String.raw`const a = "\x2d-pane\tx";`],
    ["an identity escape", String.raw`const a = "\-\-pane";`],
    ["a line continuation", 'const a = "--pa\\\nne";'],
    ["a template escape", "const a = `\\x2d-pane`;"],
    [
      "a JSX attribute, never cooked",
      String.raw`const a = <i a="--pane\x41" />;`,
    ],
  ])("matches as its literal does, spelled with %s", (_, source) => {
    expect(literalTexts(source).some((text) => PANE.test(text))).toBe(true);
    expect(mayHoldLiteral(source, PANE)).toBe(true);
  });

  it("misses when neither spelling matches", () => {
    const source = String.raw`const a = "\x2d-panel";`;
    expect(literalTexts(source).some((text) => PANE.test(text))).toBe(false);
    expect(mayHoldLiteral(source, PANE)).toBe(false);
  });
});
