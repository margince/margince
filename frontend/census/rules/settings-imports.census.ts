// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The settings catalog is split so the shell can ask where a settings entry
// lives without drawing it. `settingsnav.tsx` answers the address and
// visibility questions; `settings.tsx` renders the cards and pulls in every
// mutation hook to do it.
//
// The split holds only transitively, so this walks the graph from each entry
// point. No shell module or other screen may reach `settings.tsx`, and
// `settingsnav.tsx` may not either.

import { join } from "node:path";
import { SCREEN_ROOTS } from "../../src/testing/screenfiles";
import { importTrail, productionModules, trailsFrom } from "../imports";
import { type CensusRule, srcDir } from "../tree";

const settingsDir = join(srcDir, "features", "settings");
const settingsScreen = join(settingsDir, "settings.tsx");

export const rule: CensusRule = {
  name: "settings-imports",
  corpus: (tree) =>
    productionModules(tree.files, ["app", "lib", ...SCREEN_ROOTS]).filter(
      (file) => file.path !== settingsScreen,
    ),
  // A resolver that found no edge would pass a collapsed split.
  floors: (_corpus, tree) =>
    importTrail(
      tree,
      [settingsScreen],
      new Set([join(srcDir, "i18n", "index.tsx")]),
    ) === null
      ? ["no import path from settings.tsx to i18n/index.tsx was found"]
      : [],
  across: (corpus, tree) =>
    trailsFrom(
      tree,
      corpus.map((file) => file.path),
      new Set([settingsScreen]),
    ).map((trail) => `reaches settings.tsx: ${trail.join(" -> ")}`),
  advice:
    "Only the settings screen itself may import settings.tsx. Ask settingsnav.tsx " +
    "for an address instead, and cut the edge the path names.",
};
