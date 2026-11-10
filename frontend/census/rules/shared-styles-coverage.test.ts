// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import {
  drawsBorrowedClass,
  importsSharedSheet,
  rule,
} from "./shared-styles-coverage.census";

describe("the shared-styles-coverage census", () => {
  it("recognises the classes it is meant to hold", () => {
    expect(drawsBorrowedClass('<span className="co-row-meta">')).toBe(true);
    expect(drawsBorrowedClass('<button className="co-rowlink x">')).toBe(true);
    // A mention in prose is not a render.
    expect(drawsBorrowedClass("// co-rowlink is defined elsewhere")).toBe(
      false,
    );
    // A comment naming the stylesheet is not an import of it.
    expect(importsSharedSheet(`import "./company360.css";`)).toBe(true);
    expect(importsSharedSheet("// see company360.css for these")).toBe(false);
  });
});

describe("the shared-styles-coverage rule", () => {
  it("reports a screen that draws a shared shape without the import", () => {
    const path = join(srcDir, "features", "planted.tsx");
    const text =
      '// see company360.css\nconst row = <a className="co-rowlink" />;\n';
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toEqual([
      "src/features/planted.tsx draws a shape of company360.css without importing it",
    ]);
  });
});
