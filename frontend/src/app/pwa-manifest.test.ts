// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "node-html-parser";
import { describe, expect, it } from "vitest";
import { normalize, themes } from "../design-system/tokens-testing";

// What a browser reads before it offers to install the app. That every icon
// exists at the size it declares is sharepreview.test.ts's.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const publicDir = join(frontendRoot, "public");

interface ManifestIcon {
  src: string;
  sizes: string;
  type: string;
  purpose?: string;
}

interface Manifest {
  theme_color: string;
  background_color: string;
  icons: ManifestIcon[];
}

const manifest: Manifest = JSON.parse(
  readFileSync(join(publicDir, "manifest.webmanifest"), "utf8"),
);
const indexPage = parse(readFileSync(join(frontendRoot, "index.html"), "utf8"));

function purposeOf(icon: ManifestIcon): string {
  return icon.purpose ?? "any";
}

describe("the install manifest", () => {
  it("offers an unmasked icon and the PNGs Android masks", () => {
    const offered = manifest.icons.map(
      (icon) => `${icon.type} ${icon.sizes} ${purposeOf(icon)}`,
    );
    expect(offered).toEqual(
      expect.arrayContaining([
        "image/svg+xml any any",
        "image/png 192x192 maskable",
        "image/png 512x512 maskable",
      ]),
    );
  });

  // Not "any maskable": full-bleed art shown unmasked reads as a square slab,
  // and art drawn for "any" loses its edges to the launcher's mask.
  it("gives each icon exactly one purpose", () => {
    const shared = manifest.icons
      .filter((icon) => purposeOf(icon).trim().split(/\s+/).length !== 1)
      .map((icon) => `${icon.src}: "${purposeOf(icon)}"`);
    expect(shared).toEqual([]);
  });
});

type InstallColour = readonly [where: string, value: string, token: string];

function svgGround(file: string): string {
  const svg = readFileSync(join(publicDir, file), "utf8");
  return /<rect\b[^>]*\bfill="([^"]+)"/.exec(svg)?.[1] ?? "none";
}

function installColours(installed: Manifest): InstallColour[] {
  const metaTheme = indexPage
    .querySelector('meta[name="theme-color"]')
    ?.getAttribute("content");
  return [
    ["manifest theme_color", installed.theme_color, "--accentBrand"],
    ["index.html theme-color", metaTheme ?? "none", "--accentBrand"],
    ["manifest background_color", installed.background_color, "--bgPage"],
    ["icon.svg ground", svgGround("icon.svg"), "--accentBrand"],
  ];
}

function drift(
  colours: readonly InstallColour[],
  palette: Record<string, string>,
): string[] {
  return colours
    .filter(
      ([, value, token]) =>
        normalize(value) !== normalize(palette[token] ?? ""),
    )
    .map(
      ([where, value, token]) =>
        `${where} is ${value}, ${token} is ${palette[token]}`,
    );
}

describe("the install colours are the light tokens", () => {
  it("paints the title bar, splash and icon ground from tokens.css", () => {
    expect(drift(installColours(manifest), themes.light)).toEqual([]);
  });

  // One theme-color serves both themes only while dark keeps the brand accent.
  it("keeps one brand accent across both themes", () => {
    expect(normalize(themes.dark["--accentBrand"])).toBe(
      normalize(themes.light["--accentBrand"]),
    );
  });

  it("reports a token retuned under the files that copy it", () => {
    const colours = installColours(manifest);
    const copies = colours.filter(([, , token]) => token === "--accentBrand");
    const retuned = { ...themes.light, "--accentBrand": "#123456" };
    expect(copies.length).toBeGreaterThan(0);
    expect(drift(colours, retuned)).toHaveLength(copies.length);
  });

  it("reports a file that stops copying its token", () => {
    const repainted = { ...manifest, background_color: "#FFFFFF" };
    expect(drift(installColours(repainted), themes.light)).toContainEqual(
      expect.stringMatching(/^manifest background_color is #FFFFFF, --bgPage/),
    );
  });
});
