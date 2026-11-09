// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir } from "../tree";
import {
  CORPUS_FILE,
  materialReferenceLines,
  OWNER,
  rule,
} from "./materialsymbols.census";

describe("one module imports Material Symbols", () => {
  it.each([
    [
      "a static import",
      "probe.tsx",
      'import Info from "@material-symbols/svg-500/rounded/info-fill.svg?react";',
    ],
    [
      "a template import",
      "probe.tsx",
      `const mark = (name: string) => import(\`@material-symbols/svg-500/rounded/\${name}.svg?react\`);`,
    ],
    [
      "a require",
      "probe.cjs",
      'const info = require("@material-symbols/svg-500/rounded/info-fill.svg");',
    ],
    [
      "an import-equals",
      "probe.ts",
      'import info = require("@material-symbols/svg-500/rounded/info-fill.svg");',
    ],
    [
      "a specifier assembled from parts",
      "probe.ts",
      'const specifier = "@material-" + "symbols/svg-500/rounded/info-fill.svg";',
    ],
    [
      "an mdx page",
      "probe.mdx",
      'import Info from "@material-symbols/svg-500/rounded/info-fill.svg?react";\n\n<Info />',
    ],
    [
      "a css url",
      "probe.css",
      '.mark { mask: url("@material-symbols/svg-500/rounded/info-fill.svg"); }',
    ],
    ["a css import", "probe.css", '@import "@material-symbols/font-500";'],
    [
      "a relative path into node_modules",
      "probe.ts",
      'import "../../node_modules/@material-symbols/svg-500/rounded/info-fill.svg";',
    ],
    [
      "a glob",
      "probe.ts",
      'const marks = import.meta.glob("/node_modules/@material-symbols/svg-500/rounded/*.svg");',
    ],
    [
      "an html entry",
      "probe.html",
      '<img src="/src/design-system/material-symbols/info-fill.svg" alt="" />',
    ],
    [
      "an import of the copied files",
      "probe.tsx",
      'import Info from "../design-system/material-symbols/info-fill.svg?react";',
    ],
  ])("finds %s in a planted caller", (_shape, file, text) => {
    expect(CORPUS_FILE.test(file), `${file} is outside the corpus`).toBe(true);
    expect(materialReferenceLines(`// probe\n${text}`)).toEqual([2]);
  });

  it("reports a planted caller through its own corpus, and not the owner", () => {
    const area: Area = "src";
    const text =
      'import Info from "../design-system/material-symbols/info-fill.svg?react";';
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "planted.tsx"), area, text },
        { path: OWNER, area, text },
      ]),
    ).toEqual(["src/features/planted.tsx:1"]);
  });
});
