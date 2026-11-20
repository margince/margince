// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { declaredValues, rulesOfText } from "../../scripts/lib/css-rules";

// A control drawn smaller than a thumb gets its press target on touch from a
// `::before` layer that draws nothing. happy-dom lays nothing out and matches
// no media query, so the sheet itself is what these cases read.

const here = dirname(fileURLToPath(import.meta.url));
const TOUCH_TARGET_PX = 44;
const COARSE = "@media (pointer: coarse)";

function sheet(name: string): string {
  return readFileSync(join(here, name), "utf8").replace(
    /\/\*[\s\S]*?\*\//g,
    "",
  );
}

// The bodies of the rules inside every coarse-pointer block of one sheet.
function coarseRules(name: string) {
  const css = sheet(name);
  const blocks: string[] = [];
  for (
    let at = css.indexOf(COARSE);
    at !== -1;
    at = css.indexOf(COARSE, at + 1)
  ) {
    const open = css.indexOf("{", at);
    let depth = 0;
    for (let end = open; end < css.length; end += 1) {
      depth += css[end] === "{" ? 1 : css[end] === "}" ? -1 : 0;
      if (depth === 0) {
        blocks.push(css.slice(open + 1, end));
        break;
      }
    }
  }
  return blocks.flatMap((block) => rulesOfText(name, block));
}

function bodyOf(name: string, selector: string, rules = coarseRules(name)) {
  const found = rules.filter((rule) => rule.selector === selector);
  expect(found, `${selector} under ${COARSE} in ${name}`).not.toEqual([]);
  return found.map((rule) => rule.body).join(";");
}

// The layer's size on one axis: a length of its own, or the inset that
// centres the larger of N px and the control on it.
function side(layer: string, size: "inline-size" | "block-size"): number {
  const [length] = declaredValues(layer, size);
  const [inset] = declaredValues(layer, "inset");
  const centred = /^calc\(\(100% - max\((\d+)px, 100%\)\) \/ 2\)$/.exec(
    inset ?? "",
  );
  const px = length ?? (centred ? `${centred[1]}px` : "");
  expect(px, `${size} of the layer`).toMatch(/^\d+px$/);
  return Number.parseFloat(px);
}

describe.each([
  ["listtable.css", ".lt-select"],
  ["infotip.css", ".info-tip"],
])("the touch target of %s %s", (name, control) => {
  it("lays a 44px layer over the control under a coarse pointer", () => {
    expect(declaredValues(bodyOf(name, control), "position")).toEqual([
      "relative",
    ]);
    const layer = bodyOf(name, `${control}::before`);
    expect(declaredValues(layer, "position")).toEqual(["absolute"]);
    expect(side(layer, "inline-size")).toBeGreaterThanOrEqual(TOUCH_TARGET_PX);
    expect(side(layer, "block-size")).toBeGreaterThanOrEqual(TOUCH_TARGET_PX);
  });
});

describe("the row selection box", () => {
  // The name's link starts one gap past the box; a layer reaching past the gap
  // would take the press that opens the record.
  it("ends its target where the name's link starts", () => {
    const all = rulesOfText("listtable.css", sheet("listtable.css"));
    const [gap] = declaredValues(
      bodyOf("listtable.css", ".lt-select", all),
      "margin-inline-end",
    );
    const layer = bodyOf("listtable.css", ".lt-select::before");
    expect(declaredValues(layer, "inset-inline-end")).toEqual([
      `calc(-1 * ${gap})`,
    ]);
  });
});
