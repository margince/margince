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

const DARK_MEDIA = "@media (prefers-color-scheme: dark)";

// Every ground the style paints, by the block it is declared in: a rule after
// the media block is light-mode, wherever its text sits.
function prepaintGrounds(): { light: string[]; dark: string[] } {
  const grounds = { light: [] as string[], dark: [] as string[] };
  const open: string[] = [];
  for (const token of prepaintCss.matchAll(
    /([^{};]*)\{|\}|background:\s*([^;]+);/g,
  )) {
    if (token[0] === "}") {
      open.pop();
    } else if (token[2] !== undefined) {
      const theme = open.includes(DARK_MEDIA) ? "dark" : "light";
      grounds[theme].push(normalize(token[2]));
    } else {
      open.push(token[1].trim());
    }
  }
  return grounds;
}

describe("the first paint", () => {
  it("paints the page ground of each theme before the app loads", () => {
    expect(prepaintGrounds()).toEqual({
      light: [normalize(themes.light["--bgPage"])],
      dark: [normalize(themes.dark["--bgPage"])],
    });
  });

  // Matched without data-theme, the ground would sit under a reader's chosen
  // theme wherever the page is shorter than the window.
  it("gives way to the theme the app sets", () => {
    const selectors = [...prepaintCss.matchAll(/([^{}]+)\{/g)]
      .map((rule) => rule[1].trim())
      .filter((prelude) => !prelude.startsWith("@"));
    expect(selectors).not.toHaveLength(0);
    for (const selector of selectors) {
      expect(selector).toBe("html:not([data-theme])");
    }
  });
});
