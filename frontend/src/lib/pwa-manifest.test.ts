// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { dirname, isAbsolute, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "node-html-parser";
import { describe, expect, it } from "vitest";
import { normalize, themes } from "../design-system/tokens-testing";
import { colourAt, decodePng, type Raster } from "../testing/png";
import { parseHash, routeHash } from "./router";

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

interface ManifestShortcut {
  name: string;
  short_name: string;
  url: string;
  icons: ManifestIcon[];
}

interface Manifest {
  name: string;
  start_url: string;
  display: string;
  scope: string;
  shortcuts: ManifestShortcut[];
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

// The browser asks for whatever index.html names. A name no file in public/
// answers falls through to the SPA's index.html. The browser then parses a
// page as a manifest and reports it invalid rather than missing.
describe("the install manifest is where the page says it is", () => {
  const manifestLinks = indexPage
    .querySelectorAll("link")
    .filter((link) => link.getAttribute("rel") === "manifest");
  const href = manifestLinks[0]?.getAttribute("href");

  it("is linked once, from a root-relative path", () => {
    // Counted, not found: a second link would be discarded by a search for
    // the first, and the browser would read whichever it reached first.
    expect(manifestLinks).toHaveLength(1);
    expect(href).toBeDefined();
    expect(href?.startsWith("/")).toBe(true);
  });

  it("names a file public/ actually holds, and holds JSON", () => {
    const served = join(publicDir, (href ?? "").replace(/^\//, ""));

    // join resolves `..`, so a path can climb out of public/ and read a file
    // no browser is served.
    const inside = relative(publicDir, served);
    expect(isAbsolute(inside)).toBe(false);
    expect(inside.split("/")).not.toContain("..");

    // A page in place of the file throws here, which fails the test.
    const { name }: { name?: unknown } = JSON.parse(
      readFileSync(served, "utf8"),
    );
    expect(typeof name).toBe("string");
    expect(name).not.toBe("");
  });
});

describe("the install manifest", () => {
  it("names the app and opens it standalone at the root", () => {
    expect(manifest.name).toBe("Margince");
    expect(manifest.start_url).toBe("/");
    expect(manifest.display).toBe("standalone");
  });

  it("offers an unmasked icon and the PNGs Android masks", () => {
    const offered = manifest.icons.map(
      (icon) => `${icon.type} ${icon.sizes} ${purposeOf(icon)}`,
    );
    expect(offered).toEqual(
      expect.arrayContaining([
        "image/svg+xml any any",
        "image/png 192x192 any",
        "image/png 512x512 any",
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

describe("the install manifest shortcuts", () => {
  const origin = "https://app.example";
  const scope = new URL(manifest.scope, origin);

  it("offers a few shortcuts with short labels", () => {
    expect(manifest.shortcuts.length).toBeGreaterThanOrEqual(2);
    expect(manifest.shortcuts.length).toBeLessThanOrEqual(3);
    for (const shortcut of manifest.shortcuts) {
      expect(shortcut.short_name.length, shortcut.name).toBeLessThanOrEqual(12);
    }
  });

  it("opens only addresses inside the scope that the router answers", () => {
    for (const shortcut of manifest.shortcuts) {
      const url = new URL(shortcut.url, origin);
      expect(url.pathname.startsWith(scope.pathname), shortcut.url).toBe(true);
      const route = parseHash(url.hash);
      expect(route.screen, shortcut.url).not.toBe("not-found");
      expect(url.hash, shortcut.url).toBe(routeHash(route));
    }
  });

  it("draws each shortcut icon from a file public/ holds", () => {
    for (const shortcut of manifest.shortcuts) {
      expect(shortcut.icons.length, shortcut.name).toBeGreaterThan(0);
      for (const icon of shortcut.icons) {
        expect(existsSync(join(publicDir, icon.src)), icon.src).toBe(true);
      }
    }
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

function corners(raster: Raster) {
  const right = raster.width - 1;
  const bottom = raster.height - 1;
  return [
    colourAt(raster, 0, 0),
    colourAt(raster, right, 0),
    colourAt(raster, 0, bottom),
    colourAt(raster, right, bottom),
  ];
}

function pngIcons(purpose: string): ManifestIcon[] {
  return manifest.icons.filter(
    (icon) => icon.type === "image/png" && purposeOf(icon) === purpose,
  );
}

describe("the install icon PNGs", () => {
  const ground = normalize(themes.light["--accentBrand"]);

  // A launcher may mask down to the centred circle of 40% radius; a mark that
  // reaches past 90% of it reads as touching the mask's edge.
  it("keeps each maskable icon's mark clear of the safe zone's edge, on the brand ground", () => {
    const icons = pngIcons("maskable");
    expect(icons.length).toBeGreaterThan(0);
    for (const icon of icons) {
      const raster = decodePng(join(publicDir, icon.src));
      const clearRadius = 0.9 * 0.4 * raster.width;
      const outside: string[] = [];
      for (let y = 0; y < raster.height; y++) {
        for (let x = 0; x < raster.width; x++) {
          const fromCentre = Math.hypot(
            x + 0.5 - raster.width / 2,
            y + 0.5 - raster.height / 2,
          );
          const { hex, alpha } = colourAt(raster, x, y);
          if (
            fromCentre > clearRadius &&
            (normalize(hex) !== ground || alpha !== 255)
          ) {
            outside.push(`${x},${y} ${hex}`);
          }
        }
      }
      expect(
        corners(raster).map(({ hex }) => normalize(hex)),
        icon.src,
      ).toEqual(Array(4).fill(ground));
      expect(outside.slice(0, 5), icon.src).toEqual([]);
    }
  });

  it("keeps the rounded square's corners clear on each unmasked icon", () => {
    const icons = pngIcons("any");
    expect(icons.length).toBeGreaterThan(0);
    for (const icon of icons) {
      const alphas = corners(decodePng(join(publicDir, icon.src))).map(
        ({ alpha }) => alpha,
      );
      expect(alphas, icon.src).toEqual([0, 0, 0, 0]);
    }
  });
});
