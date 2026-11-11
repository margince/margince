// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { createHash } from "node:crypto";
import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { cleanup, render } from "@testing-library/react";
import { Info } from "lucide-react";
import { afterEach, describe, expect, it } from "vitest";
import { stable } from "../format/collate";
import { type Glyph, InfoFilled, WarningFilled } from "./materialsymbols";

afterEach(cleanup);

const frontendRoot = resolve(__dirname, "../..");
const srcDir = join(frontendRoot, "src");
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

describe("the copied Material files", () => {
  it("are byte for byte the published ones, and only those", () => {
    expect(readdirSync(COPIES).sort(stable)).toEqual(
      Object.keys(PUBLISHED).sort(stable),
    );
    for (const [file, sha256] of Object.entries(PUBLISHED)) {
      const digest = createHash("sha256")
        .update(readFileSync(join(COPIES, file)))
        .digest("hex");
      expect(digest, `${file} differs from the published file`).toBe(sha256);
    }
  });
});
