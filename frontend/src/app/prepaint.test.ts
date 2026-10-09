// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "node-html-parser";
import { describe, expect, it } from "vitest";
import { normalize, themes } from "../design-system/tokens-testing";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const indexPage = parse(readFileSync(join(frontendRoot, "index.html"), "utf8"));
// What index.html paints before app.css loads.
const prepaintCss = indexPage
  .querySelectorAll("head style")
  .map((style) => style.text)
  .join("\n");

// Split at the one dark-mode media query.
function prepaintGrounds(): { light: string; dark: string } {
  const [light, dark] = prepaintCss.split(
    "@media (prefers-color-scheme: dark)",
  );
  const ground = (part: string | undefined) =>
    /background:\s*([^;]+);/.exec(part ?? "")?.[1] ?? "";
  return { light: ground(light), dark: ground(dark) };
}

describe("the first paint", () => {
  it("paints the page ground of each theme before the app loads", () => {
    const grounds = prepaintGrounds();
    expect(normalize(grounds.light)).toBe(normalize(themes.light["--bgPage"]));
    expect(normalize(grounds.dark)).toBe(normalize(themes.dark["--bgPage"]));
  });

  // Matched without data-theme, the ground would sit under a reader's chosen
  // theme wherever the page is shorter than the window.
  it("gives way to the theme the app sets", () => {
    const selectors = [...prepaintCss.matchAll(/([^{}]+)\{/g)]
      .map((rule) => rule[1].trim())
      .filter((prelude) => !prelude.startsWith("@"));
    expect(selectors).not.toHaveLength(0);
    expect(new Set(selectors)).toEqual(new Set(["html:not([data-theme])"]));
  });
});
