// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  importPathTo,
  productionModulesUnder,
} from "../../scripts/lib/import-reach";

// The settings catalog is split in two so the shell can ask where a settings
// entry lives without paying to draw it. `settingsnav.tsx` answers the address
// and visibility questions; `settings.tsx` renders the cards and pulls in every
// mutation hook and roughly a hundred and fifty imports to do it.
//
// That split only buys anything while it holds TRANSITIVELY. A direct-import
// grep would pass the day someone adds `settingsnav -> somecard -> settings`,
// and the shell would silently go back to loading the whole settings screen to
// render a nav rail. So this walks the graph from each entry point and fails on
// the reachable set, not on the first hop.
//
// Two obligations, both real failures we would otherwise ship blind:
//
//  1. No file under `src/app/**` may reach `settings.tsx`. Those are the always-
//     loaded shell modules; reaching the screen defeats its lazy chunk for every
//     page in the product, not only settings.
//  2. `settingsnav.tsx` may not reach `settings.tsx`. It is the light half by
//     construction, and a cycle through it re-couples every one of its readers.

const screensDir = dirname(fileURLToPath(import.meta.url));
const srcRoot = resolve(screensDir, "..");
const settingsScreen = join(screensDir, "settings.tsx");
const settingsNav = join(screensDir, "settingsnav.tsx");

/** The shortest import path from `entry` to `target`, or null. */
function pathTo(entry: string, target: string): string[] | null {
  const trail = importPathTo(entry, new Set([target]));
  return trail === null ? null : trail.map((file) => relative(srcRoot, file));
}

/**
 * Every production module that must not reach the settings screen, DERIVED
 * rather than listed. A hand-kept list is a second copy of "who asks a settings
 * question", and it goes stale the first time somebody adds a module — silently,
 * because a gate that reads a smaller tree still reports PASS.
 *
 * Two trees qualify. `src/app/**` is the always-loaded shell. `src/screens/**`
 * is every other screen: a screen that only wants an ADDRESS must not drag the
 * settings cards into its own chunk, which is what `worklist.copy.ts` did —
 * putting the whole settings screen behind Brief, the default landing page.
 */
const settingsOwnModules = new Set([settingsScreen, settingsNav]);
const shellEntryPoints = [
  ...productionModulesUnder(join(srcRoot, "app")),
  ...productionModulesUnder(join(srcRoot, "screens")),
]
  .filter((file) => !settingsOwnModules.has(file))
  .map((file) => relative(srcRoot, file))
  .sort();

describe("the settings nav split holds transitively", () => {
  it.each(shellEntryPoints)("%s does not reach settings.tsx", (entry) => {
    const trail = pathTo(join(srcRoot, entry), settingsScreen);
    expect(
      trail,
      trail === null ? "" : `import path: ${trail.join(" -> ")}`,
    ).toBeNull();
  });

  it("settingsnav.tsx does not reach settings.tsx", () => {
    const trail = pathTo(settingsNav, settingsScreen);
    expect(
      trail,
      trail === null ? "" : `import path: ${trail.join(" -> ")}`,
    ).toBeNull();
  });

  // The walk is only worth its runtime if it can actually see an edge. Without
  // this, a resolver that silently returned null for every specifier would
  // report PASS on a tree where the split had completely collapsed.
  it("finds a path that does exist, so a green result means something", () => {
    const trail = pathTo(settingsScreen, join(srcRoot, "i18n/index.tsx"));
    expect(trail).not.toBeNull();
    expect(trail?.[0]).toBe("screens/settings.tsx");
  });
});
