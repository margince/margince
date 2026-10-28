// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { join, relative, resolve } from "node:path";
import { cleanup, render } from "@testing-library/react";
import { Info } from "lucide-react";
import { afterEach, describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  moduleSpecifiers,
  parseSource,
  sourceTextAt,
} from "../../scripts/lib/source-tree";
import { type Glyph, InfoFilled, WarningFilled } from "./materialsymbols";

afterEach(cleanup);

const frontendRoot = resolve(__dirname, "../..");
const srcDir = join(frontendRoot, "src");
const extensionsRoot = join(frontendRoot, "..", "extensions");
const OWNER = join(srcDir, "design-system", "materialsymbols.tsx");

function materialImports(path: string, text: string): string[] {
  if (!text.includes("@material-symbols/")) return [];
  return moduleSpecifiers(parseSource(path, text), "all").filter((specifier) =>
    specifier.startsWith("@material-symbols/"),
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
  it("finds the import in a planted caller", () => {
    expect(
      materialImports(
        join(srcDir, "features", "probe.tsx"),
        'import Info from "@material-symbols/svg-500/rounded/info-fill.svg?react";\n' +
          'const lazy = () => import("@material-symbols/svg-500/rounded/error-fill.svg?react");',
      ),
    ).toHaveLength(2);
  });

  it("finds none outside materialsymbols.tsx", { timeout: 60_000 }, () => {
    const files = filesUnder(srcDir).concat(
      extensionFrontendFiles(extensionsRoot),
    );
    expect(files.length).toBeGreaterThan(100);
    expect(
      materialImports(OWNER, sourceTextAt(OWNER)).length,
      "the owner imports nothing, so the census reads the wrong module",
    ).toBeGreaterThan(0);

    const strays = files
      .filter((file) => file !== OWNER)
      .flatMap((file) =>
        materialImports(file, sourceTextAt(file)).map(
          (specifier) => `${relative(frontendRoot, file)}: ${specifier}`,
        ),
      );
    expect(
      strays,
      "Import the glyph from src/design-system/materialsymbols.tsx; add it there if it is missing.",
    ).toEqual([]);
  });
});
