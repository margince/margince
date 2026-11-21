// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { createContext, runInContext } from "node:vm";
import { parse } from "node-html-parser";
import { afterEach, describe, expect, it, vi } from "vitest";
import { normalize, themes } from "../design-system/tokens-testing";
import { STORAGE_KEYS } from "../lib/storage";
import { resolveTheme, THEME_CHOICES, type Theme } from "../lib/theme";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const indexPage = parse(readFileSync(join(frontendRoot, "index.html"), "utf8"));
// What index.html paints before app.css loads.
const prepaintCss = indexPage
  .querySelectorAll("head style")
  .map((style) => style.text)
  .join("\n");
const themeScripts = indexPage
  .querySelectorAll("head script")
  .filter((script) => !script.hasAttribute("src"));

const DARK_MEDIA = "@media (prefers-color-scheme: dark)";

type Rule = { selector: string; darkOnly: boolean; background?: string };

function prepaintRules(): Rule[] {
  const rules: Rule[] = [];
  const open: string[] = [];
  for (const token of prepaintCss.matchAll(
    /([^{};]*)\{|\}|background:\s*([^;]+);/g,
  )) {
    if (token[0] === "}") {
      open.pop();
    } else if (token[2] !== undefined) {
      const rule = rules.at(-1);
      if (rule) rule.background = token[2].trim();
    } else {
      const prelude = token[1].trim();
      open.push(prelude);
      if (!prelude.startsWith("@"))
        rules.push({ selector: prelude, darkOnly: open.includes(DARK_MEDIA) });
    }
  }
  return rules;
}

type State = { stored: Theme | undefined; osDark: boolean };

// Whether a selector matches <html> in a state, and how specific it is. A
// selector this does not know fails the test rather than being misjudged.
function match(selector: string, { stored }: State): number | undefined {
  const known: Record<string, [boolean, number]> = {
    html: [true, 1],
    'html[data-theme="light"]': [stored === "light", 11],
    'html[data-theme="dark"]': [stored === "dark", 11],
    "html:not([data-theme])": [stored === undefined, 11],
  };
  const found = known[selector];
  if (!found) throw new Error(`the prepaint test cannot judge ${selector}`);
  return found[0] ? found[1] : undefined;
}

function groundIn(state: State): string | undefined {
  let winner: { specificity: number; background: string } | undefined;
  for (const rule of prepaintRules()) {
    if (rule.background === undefined || (rule.darkOnly && !state.osDark))
      continue;
    const specificity = match(rule.selector, state);
    if (specificity !== undefined && specificity >= (winner?.specificity ?? 0))
      winner = { specificity, background: rule.background };
  }
  return winner?.background;
}

describe("the first paint", () => {
  const fallback = (ground: string | undefined) =>
    normalize(/^var\(--bgPage,\s*([^)]+)\)$/.exec(ground ?? "")?.[1] ?? "");

  it.each([
    { stored: undefined, osDark: false, theme: "light" },
    { stored: undefined, osDark: true, theme: "dark" },
    { stored: "light", osDark: true, theme: "light" },
    { stored: "dark", osDark: false, theme: "dark" },
    { stored: "light", osDark: false, theme: "light" },
    { stored: "dark", osDark: true, theme: "dark" },
  ] as const)(
    "paints the $theme ground for a stored $stored on a computer in dark mode: $osDark",
    ({ theme, ...state }) => {
      expect(fallback(groundIn(state))).toBe(
        normalize(themes[theme]["--bgPage"]),
      );
    },
  );

  // A fixed colour would sit under the app's own ground wherever the page is
  // shorter than the window, and outlive a theme switch.
  it("follows --bgPage once app.css defines it", () => {
    const grounds = prepaintRules().flatMap((rule) => rule.background ?? []);
    expect(grounds).not.toHaveLength(0);
    for (const ground of grounds) expect(ground).toMatch(/^var\(--bgPage,/);
  });
});

describe("the stored theme, before the first paint", () => {
  afterEach(() => vi.unstubAllGlobals());

  function storage(value: string | null): Pick<Storage, "getItem"> {
    return {
      getItem: (name) => (name === STORAGE_KEYS.theme.name ? value : null),
    };
  }

  function themeSetBy(store: Pick<Storage, "getItem">): string | undefined {
    const root = { dataset: {} as Record<string, string> };
    runInContext(
      themeScripts.map((script) => script.text).join("\n"),
      createContext({
        localStorage: store,
        document: { documentElement: root },
      }),
    );
    return root.dataset.theme;
  }

  function appTheme(stored: string | null, osDark: boolean): Theme {
    vi.stubGlobal("localStorage", storage(stored));
    vi.stubGlobal("window", { matchMedia: () => ({ matches: osDark }) });
    return resolveTheme();
  }

  it("runs as one classic script in the head", () => {
    expect(themeScripts).toHaveLength(1);
    expect(themeScripts[0].getAttribute("type")).toBeUndefined();
    expect(STORAGE_KEYS.theme.area).toBe("local");
  });

  // The app paints the same theme when main.tsx starts, so nothing flips.
  it.each([...THEME_CHOICES, "sepia", "", null])(
    "sets what the app resolves for a stored %s, and leaves the computer's mode to the stylesheet",
    (stored) => {
      const set = themeSetBy(storage(stored));
      const followsComputer =
        appTheme(stored, false) !== appTheme(stored, true);
      if (followsComputer) {
        expect(set).toBeUndefined();
      } else {
        expect(set).toBe(appTheme(stored, false));
      }
    },
  );

  it("leaves the theme to the computer when storage is refused", () => {
    const refused = {
      getItem: () => {
        throw new DOMException("refused", "SecurityError");
      },
    };
    expect(themeSetBy(refused)).toBeUndefined();
  });
});
