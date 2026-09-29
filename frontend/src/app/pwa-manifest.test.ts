// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "node-html-parser";
import { describe, expect, it } from "vitest";
import { normalize, themes } from "../design-system/tokens-testing";

// What a browser reads before it offers to install the app, read off the files
// it is served. The PNGs are baked by scripts/gen-pwa-icons.mjs from the SVGs.

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

const PNG_SIGNATURE = Buffer.from([
  0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
]);
// PNG colour types 0 and 2: greyscale and truecolour, neither with alpha.
const OPAQUE_COLOUR_TYPES = [0, 2];

interface PngHeader {
  size: string;
  colourType: number;
}

function pngHeader(src: string): PngHeader {
  const bytes = readFileSync(join(publicDir, src));
  if (!bytes.subarray(0, 8).equals(PNG_SIGNATURE)) {
    throw new Error(`${src} is not a PNG`);
  }
  if (bytes.toString("latin1", 12, 16) !== "IHDR") {
    throw new Error(`${src} does not open with its IHDR chunk`);
  }
  return {
    size: `${bytes.readUInt32BE(16)}x${bytes.readUInt32BE(20)}`,
    colourType: bytes[25],
  };
}

function linkHref(selector: string): string {
  const href = indexPage.querySelector(selector)?.getAttribute("href");
  if (!href) throw new Error(`index.html has no ${selector}`);
  return href;
}

function purposeOf(icon: ManifestIcon): string {
  return icon.purpose ?? "any";
}

describe("the install manifest", () => {
  it("lists only icons public/ serves", () => {
    const missing = manifest.icons
      .map((icon) => icon.src)
      .filter((src) => !existsSync(join(publicDir, src)));
    expect(manifest.icons.length).toBeGreaterThan(0);
    expect(missing).toEqual([]);
  });

  it("declares every PNG icon at the size its pixels are", () => {
    const pngs = manifest.icons.filter((icon) => icon.type === "image/png");
    expect(pngs.length).toBeGreaterThan(0);
    for (const icon of pngs) {
      expect(pngHeader(icon.src).size, icon.src).toBe(icon.sizes);
    }
  });

  it("offers the PNGs Chrome installs from and Android masks", () => {
    const offered = manifest.icons
      .filter((icon) => icon.type === "image/png")
      .map((icon) => `${icon.sizes} ${purposeOf(icon)}`);
    expect(offered).toEqual(
      expect.arrayContaining([
        "192x192 any",
        "512x512 any",
        "512x512 maskable",
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

describe("the icons index.html links", () => {
  it("links only files public/ serves", () => {
    const links = indexPage.querySelectorAll('link[href^="/"]');
    const rels = new Set(links.map((link) => link.getAttribute("rel")));
    expect([...rels]).toEqual(
      expect.arrayContaining(["icon", "apple-touch-icon", "manifest"]),
    );
    const missing = links
      .map((link) => link.getAttribute("href") ?? "")
      .filter((href) => !existsSync(join(publicDir, href)));
    expect(missing).toEqual([]);
  });

  // iOS paints every transparent pixel of a touch icon black.
  it("gives iOS an opaque 180px touch icon", () => {
    const header = pngHeader(linkHref('link[rel="apple-touch-icon"]'));
    expect(header.size).toBe("180x180");
    expect(OPAQUE_COLOUR_TYPES).toContain(header.colourType);
  });

  it("falls back to a PNG favicon at the size it declares", () => {
    const selector = 'link[rel="icon"][type="image/png"]';
    const declared = indexPage.querySelector(selector)?.getAttribute("sizes");
    expect(pngHeader(linkHref(selector)).size).toBe(declared);
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
    [
      "icon-maskable.svg ground",
      svgGround("icon-maskable.svg"),
      "--accentBrand",
    ],
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
