// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { extensionsDir, plantedFindings, srcDir } from "../tree";
import { type InlineLayout, inlineLayoutIn, rule } from "./inlinelayout.census";

// The detector must see each shape, or a clean tree and a blind reader look
// the same.
describe("the detector", () => {
  const read = (source: string): InlineLayout[] =>
    inlineLayoutIn("fixture.tsx", parseSource("fixture.tsx", source));

  it("reads a static rule, on one line and spread over many", () => {
    expect(
      read('<p style={{ marginTop: "var(--space-2)" }}>{note}</p>'),
    ).toEqual([{ where: "fixture.tsx", line: 1, property: "marginTop" }]);
    expect(
      read(
        '<div\n  style={{\n    display: "flex",\n    gap: "var(--space-2)",\n    alignItems: "center",\n  }}\n/>',
      ).map((one) => `${one.line}:${one.property}`),
    ).toEqual(["3:display", "4:gap", "5:alignItems"]);
  });

  it("reads a literal however the parser hands it over", () => {
    expect(read("<input style={{ width: 180 }} />")).toHaveLength(1);
    expect(read("<div style={{ padding: `0` }} />")).toHaveLength(1);
    expect(read("<div style={{ marginLeft: -4 }} />")).toHaveLength(1);
    expect(read('<div style={{ marginTop: ("4px") }} />')).toHaveLength(1);
    expect(read('<div style={{ marginTop: "4px" as const }} />')).toHaveLength(
      1,
    );
    expect(read('<div style={{ ["marginTop"]: "4px" }} />')).toHaveLength(1);
    expect(read('<div style={{ "margin-top": "4px" }} />')).toHaveLength(1);
  });

  it("reads through every wrapper an element can hand its style", () => {
    const shapes = [
      '<div style={open ? { gap: "4px" } : undefined} />',
      '<div style={open && { gap: "4px" }} />',
      '<div style={given ?? { gap: "4px" }} />',
      '<div style={Object.assign({}, { gap: "4px" })} />',
      '<div style={{ gap: "4px" } as CSSProperties} />',
      '<div style={{ gap: "4px" } satisfies CSSProperties} />',
      '<div style={({ gap: "4px" })} />',
      '<div style={{ ...{ gap: "4px" } }} />',
      '<div style={{ ...(tight && { gap: "4px" }) }} />',
      '<div style={merge(base, { gap: "4px" })} />',
    ];
    expect(
      shapes.filter((shape) => read(shape).length !== 1),
      "a wrapper hid the rule",
    ).toEqual([]);
  });

  it("reads every road a style reaches an element by", () => {
    const roads = [
      '<div {...{ style: { marginTop: "4px" } }} />',
      'const props = { style: { gap: "8px" } };\n<div {...props} />',
      'createElement("div", { style: { padding: 0 } })',
      'const hoisted = { padding: 0 };\nReact.createElement("div", { style: hoisted })',
      'jsx("div", { style: { gap: "8px" } })',
    ];
    expect(
      roads.filter((road) => read(road).length !== 1),
      "a style reached its element unread",
    ).toEqual([]);
  });

  it("leaves a props object the render built alone", () => {
    expect(
      read("const frameProps = propsFor(frame);\n<div {...frameProps} />"),
    ).toEqual([]);
    expect(read("<div {...frameProps} />")).toEqual([]);
    expect(read('createElement("div", propsFor(frame))')).toEqual([]);
  });

  it("reads a style object hoisted to a name of its own", () => {
    expect(
      read('const row = { marginTop: "4px" };\n<div style={row} />'),
    ).toEqual([{ where: "fixture.tsx", line: 1, property: "marginTop" }]);
    expect(
      read('const sheet = { row: { gap: "4px" } };\n<div style={sheet.row} />'),
    ).toHaveLength(1);
  });

  it("reads a name in its own scope, not a same-named one beside it", () => {
    const twoRows = (styled: string, unstyled: string) =>
      [
        `function A() { const row = ${styled}; return <div style={row} />; }`,
        `function B() { const row = ${unstyled}; return <i data-row={row} />; }`,
      ].join("\n");
    expect(
      read(twoRows('{ marginTop: "4px" }', '{ color: "red" }')),
    ).toHaveLength(1);
    expect(read(twoRows('{ color: "red" }', '{ marginTop: "4px" }'))).toEqual(
      [],
    );
  });

  it("reads a declared style with no element in sight", () => {
    expect(
      read('const row: CSSProperties = { marginTop: "4px" };'),
    ).toHaveLength(1);
    expect(
      read('const row: React.CSSProperties = { marginTop: "4px" };'),
    ).toHaveLength(1);
  });

  it("counts one rule once, however many ways it is reachable", () => {
    expect(
      read(
        'const row: CSSProperties = { marginTop: "4px" };\n<div style={row} />',
      ),
    ).toHaveLength(1);
  });

  it("leaves a value the render computed alone", () => {
    expect(read("<col style={{ width: widthOf(column) }} />")).toEqual([]);
    expect(read("<div style={{ width: frame.width }} />")).toEqual([]);
    // biome-ignore lint/suspicious/noTemplateCurlyInString: this string is source the walker parses, and its substitution is the case under test
    expect(read("<div style={{ top: `${at.top}px` }} />")).toEqual([]);
    // biome-ignore lint/suspicious/noTemplateCurlyInString: a bar filled to a percentage the render worked out, parsed as source
    expect(read("<span style={{ width: `${filled}%` }} />")).toEqual([]);
    expect(read("<div style={{ marginTop: tight ? 0 : 8 }} />")).toEqual([]);
    expect(read("<div style={{ width }} />")).toEqual([]);
    expect(read("<div style={{ ...frame }} />")).toEqual([]);
    expect(read("<div style={frameStyle} />")).toEqual([]);
  });

  it("leaves a property that lays nothing out alone", () => {
    expect(read('<span style={{ userSelect: "none" }} />')).toEqual([]);
    expect(read('<p style={{ color: "var(--dangerText)" }} />')).toEqual([]);
    expect(
      read('<text style={{ fontWeight: "var(--fontWeightBold)" }} />'),
    ).toEqual([]);
    expect(read('<div style={{ opacity: "0" }} />')).toEqual([]);
  });

  it("reads the whole family, not the properties somebody listed", () => {
    const families = [
      '<div style={{ marginInlineStart: "0" }} />',
      '<div style={{ gridColumn: "1" }} />',
      '<div style={{ position: "relative" }} />',
      '<div style={{ overflowX: "hidden" }} />',
      '<div style={{ float: "left" }} />',
      "<div style={{ order: 2 }} />",
      '<div style={{ boxSizing: "border-box" }} />',
      '<div style={{ aspectRatio: "16 / 9" }} />',
      '<div style={{ transform: "translateY(-2px)" }} />',
      '<div style={{ translate: "0 -2px" }} />',
      "<div style={{ zIndex: 2 }} />",
      '<div style={{ inlineSize: "8rem" }} />',
      '<div style={{ blockSize: "8rem" }} />',
      '<div style={{ minInlineSize: "8rem" }} />',
      '<div style={{ maxInlineSize: "8rem" }} />',
      '<div style={{ minBlockSize: "8rem" }} />',
      '<div style={{ maxBlockSize: "8rem" }} />',
      '<div style={{ marginInline: "auto" }} />',
      '<div style={{ paddingBlock: "4px" }} />',
      "<div style={{ insetInlineStart: 0 }} />",
    ];
    expect(
      families.filter((one) => read(one).length !== 1),
      "a layout family went unread",
    ).toEqual([]);
  });

  it("reads a unit's own dialect", () => {
    const jsx = inlineLayoutIn(
      "screen.jsx",
      parseSource("screen.jsx", '<div style={{ gap: "4px" }} />'),
    );
    expect(jsx).toHaveLength(1);
  });
});

describe("the inlinelayout rule", () => {
  it("reports a planted inline margin through its own corpus and visit", () => {
    const text = 'export const F = () => <p style={{ marginTop: "4px" }} />;\n';
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "planted.tsx"), area: "src", text },
        {
          path: join(extensionsDir, "unit", "frontend", "screen.tsx"),
          area: "extensions",
          text,
        },
        {
          path: join(srcDir, "features", "planted.stories.tsx"),
          area: "src",
          text,
        },
      ]),
    ).toEqual([
      "src/features/planted.tsx:1: `marginTop` is laid out inline, a class in the screen's sheet, or `Stack`/`Row` from design-system/stack.tsx",
      "../extensions/unit/frontend/screen.tsx:1: `marginTop` is laid out inline, a class in the screen's sheet, or `Stack`/`Row` from design-system/stack.tsx",
    ]);
  });
});
