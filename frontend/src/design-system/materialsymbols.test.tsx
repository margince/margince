// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
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

// Raw text, not a parsed import: a template specifier, a `require`, an `.mdx`
// page or a CSS `url()` names the files where no string-literal import shows.
const MATERIAL_REFERENCE = /material-symbols|@material-["'`$]/g;
const CORPUS_FILE = /\.(ts|tsx|mts|cts|js|jsx|mjs|cjs|mdx|css)$/;

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
    );
    expect(files.length).toBeGreaterThan(100);
    expect(files.some((file) => file.endsWith(".css"))).toBe(true);
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
      "Import the glyph from src/design-system/materialsymbols.tsx; a missing one is an SVG copied into material-symbols/ and imported there.",
    ).toEqual([]);
  });
});
