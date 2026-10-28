// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { createHash } from "node:crypto";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { cleanup, render } from "@testing-library/react";
import { Info } from "lucide-react";
import { afterEach, describe, expect, it } from "vitest";
import {
  extensionFilesMatching,
  filesMatching,
  sourceTextAt,
} from "../../scripts/lib/source-tree";
import { type Glyph, InfoFilled, WarningFilled } from "./materialsymbols";

afterEach(cleanup);

const frontendRoot = resolve(__dirname, "../..");
const srcDir = join(frontendRoot, "src");
const extensionsRoot = join(frontendRoot, "..", "extensions");
const OWNER = join(srcDir, "design-system", "materialsymbols.tsx");
const COPIES = join(srcDir, "design-system", "material-symbols");

// Each copied file's sha256 as published in @material-symbols/svg-500 0.48.1.
const PUBLISHED: Readonly<Record<string, string>> = {
  LICENSE: "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
  "check_circle-fill.svg":
    "6403fa8db865a30ba8fe4d22cb293d7df2cee74da774e2d119e401e15bb90bc2",
  "error-fill.svg":
    "5b16c9672a4ec4d201a37028f6ff3821994552a93fbd806e508c8c5987c81e2c",
  "info-fill.svg":
    "2181d79146e81135a1af0d3a98fd3fcc08c87f0f7162656b57e42b08a4ff78d8",
  "lightbulb_2-fill.svg":
    "51c84ce4cc99bb4289550b6814098a804eb7dac923b85f10311ba19d07753132",
  "warning-fill.svg":
    "f13d716ece4e68f885df9519aa586c223735df24f13b80ee856b303ea79ee3a9",
};

// Raw text, not a parsed import: a template specifier, a `require`, an `.mdx`
// page or a CSS `url()` names the files where no string-literal import shows.
const MATERIAL_REFERENCE = /material-symbols|@material-["'`$]/g;
const CORPUS_FILE = /\.(ts|tsx|mts|cts|js|jsx|mjs|cjs|mdx|css|html)$/;

function materialReferenceLines(text: string): number[] {
  return [...text.matchAll(MATERIAL_REFERENCE)].map(
    (match) => text.slice(0, match.index).split("\n").length,
  );
}

describe("a Material symbol", () => {
  it("draws in the ink around it, at the size it is given", () => {
    const { container } = render(<InfoFilled size={16} />);
    const svg = container.querySelector("svg");
    expect(svg).toHaveAttribute("fill", "currentColor");
    expect(svg).toHaveAttribute("width", "16");
    expect(svg).toHaveAttribute("height", "16");
    expect(svg).toHaveAttribute("viewBox", "0 -960 960 960");
    expect(svg).toHaveAttribute("aria-hidden", "true");
  });

  it("takes lucide's default size when it is given none, not the file's 48", () => {
    const { container } = render(<WarningFilled />);
    expect(container.querySelector("svg")).toHaveAttribute("width", "24");
  });

  it("names its glyph and passes the caller's class through", () => {
    const { container } = render(<InfoFilled className="co-mark" />);
    const svg = container.querySelector("svg");
    expect(svg).toHaveAttribute("data-symbol", "info");
    expect(svg).toHaveAttribute("class", "co-mark");
  });

  it("fills the same slot a lucide glyph fills", () => {
    const slot: ReadonlyArray<readonly [string, Glyph]> = [
      ["lucide", Info],
      ["material", InfoFilled],
    ];
    const { container } = render(
      <span>
        {slot.map(([source, Mark]) => (
          <Mark key={source} size={16} />
        ))}
      </span>,
    );
    expect(container.querySelectorAll('svg[width="16"]')).toHaveLength(2);
  });
});

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

  it("finds none outside materialsymbols.tsx", { timeout: 60_000 }, () => {
    const files = filesMatching(srcDir, CORPUS_FILE).concat(
      extensionFilesMatching(extensionsRoot, CORPUS_FILE),
      join(frontendRoot, "index.html"),
    );
    expect(files.length).toBeGreaterThan(100);
    expect(files.some((file) => file.endsWith(".css"))).toBe(true);
    expect(files.some((file) => file.endsWith(".html"))).toBe(true);
    const owner = sourceTextAt(OWNER);
    expect(
      materialReferenceLines(owner).length,
      "the owner imports no mark, so the census reads the wrong module",
    ).toBeGreaterThan(0);
    expect(owner, "the owner imports the npm package").not.toMatch(
      /["'`]@material-symbols\//,
    );

    const strays = files
      .filter((file) => file !== OWNER && file !== __filename)
      .flatMap((file) =>
        materialReferenceLines(sourceTextAt(file)).map(
          (line) => `${relative(frontendRoot, file)}:${line}`,
        ),
      );
    expect(
      strays,
      "Import the glyph from src/design-system/materialsymbols.tsx, which says how to add a missing one.",
    ).toEqual([]);
  });
});

describe("the copied Material files", () => {
  it("are byte for byte the published ones, and only those", () => {
    expect(readdirSync(COPIES).sort()).toEqual(Object.keys(PUBLISHED).sort());
    for (const [file, sha256] of Object.entries(PUBLISHED)) {
      const digest = createHash("sha256")
        .update(readFileSync(join(COPIES, file)))
        .digest("hex");
      expect(digest, `${file} differs from the published file`).toBe(sha256);
    }
  });
});
