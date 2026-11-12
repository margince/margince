// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Storybook's settings tree and the product's settings sidebar say the same
// words, or the workbench describes a product that no longer exists.
//
// This does not hold that the page a story names is the page whose dispatch
// arm renders its card. settings.tsx keeps that map as a switch, and a card
// nested in another card cannot be read from it.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { storySuffixes } from "../../scripts/lib/story-files";
import { defaultExportObject, literalAt } from "../../scripts/lib/story-title";
import { en } from "../../src/i18n/en";
import { SETTINGS_PAGES } from "../../src/lib/settingscatalog";
import { type CensusFile, type CensusRule, frontendRoot } from "../tree";

// The group/page pairs the catalog declares, as the sidebar spells them. Pairs,
// because `Settings/AI/Capture/Card` names a real group and a real page and is
// still wrong: Capture belongs to Data.
const words: Readonly<Record<string, string>> = en;
const PAGE_PATHS = new Set(
  SETTINGS_PAGES.map(
    (page) =>
      `${words[`settings.group.${page.group}`]}/${words[`settings.tab.${page.id}`]}`,
  ),
);

// Stories about a settings surface rather than a card on a page, named one by
// one so `Settings/Nonsense` is not exempt. The settings home is no member of
// SETTINGS_PAGES.
const SURFACE_STORIES = new Set([
  "Settings/Settings screen",
  "Settings/Settings navigation",
  "Settings/Settings home",
]);

// The one group that is not a catalog group: cards from different pages side
// by side, or one control two pages mount. Members are listed, so a new one is
// an edit to this list, and one that stops crossing pages fails here.
const ACROSS_PAGES = "Across pages";
const ACROSS_PAGE_STORIES = new Set([
  "Settings/Across pages/Rates and model costs",
  "Settings/Across pages/Refresh from sources",
  "Settings/Across pages/Signature fields",
  // A unit's declared secret scope decides which of two pages offers it, so
  // filing the card under either would say the other page does not.
  "Settings/Across pages/Units offered in settings",
]);

// Pinned rather than floored: a story retitled away from `Settings/` drops out
// of the corpus, and only an exact count notices it go.
export const SETTINGS_STORY_COUNT = 238;

// Read off Storybook's own globs, so a new kind of story file joins the corpus.
const mainPath = join(frontendRoot, ".storybook", "main.ts");
const suffixes = storySuffixes(mainPath, readFileSync(mainPath, "utf8")) ?? [];

/** The sidebar path a story file claims at its default export, or null. */
export function titleOf(file: CensusFile): string | null {
  if (!file.source) return null;
  const title = literalAt(defaultExportObject(file.source), ["title"]);
  return title !== undefined && ts.isStringLiteralLike(title)
    ? title.text
    : null;
}

/** Why a settings story's title is misfiled, or null when it is not. */
export function misfiling(title: string): string | null {
  if (SURFACE_STORIES.has(title)) return null;
  const segments = title.split("/");
  if (segments[1] === ACROSS_PAGES) {
    return ACROSS_PAGE_STORIES.has(title)
      ? null
      : "is filed across pages but not listed in ACROSS_PAGE_STORIES";
  }
  if (segments.length !== 4) {
    return "is not Settings/<Group>/<Page>/<Card>";
  }
  return PAGE_PATHS.has(`${segments[1]}/${segments[2]}`)
    ? null
    : `files under ${segments[1]}/${segments[2]}, which the catalog does not declare`;
}

// A card's story can sit beside a component outside features/, so every story
// under src/ is read. An .mdx docs page is outside the parsed tree.
const isSettingsStory = (file: CensusFile) =>
  file.area === "src" &&
  suffixes.some((suffix) => file.path.endsWith(suffix)) &&
  (titleOf(file)?.startsWith("Settings/") ?? false);

export const rule: CensusRule = {
  name: "settingsstories",
  corpus: (tree) => tree.files.filter(isSettingsStory),
  floors: (corpus) =>
    corpus.length === SETTINGS_STORY_COUNT
      ? []
      : [
          `read ${corpus.length} settings stories, expected ${SETTINGS_STORY_COUNT}; update SETTINGS_STORY_COUNT if the change is meant`,
        ],
  visit: (file) => {
    const title = titleOf(file) ?? "";
    const wrong = misfiling(title);
    return wrong === null ? [] : [`${file.rel}: "${title}" ${wrong}`];
  },
  advice:
    "File a settings story as Settings/<Group>/<Page>/<Card>, with the group " +
    "and page the sidebar shows for the page that renders the card.",
};
