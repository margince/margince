// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";
import { storyTitle } from "../../scripts/lib/story-title";
import { translate } from "../i18n";
import { SETTINGS_PAGES, type SettingsPageId } from "./settingscatalog";

// Storybook's settings tree and the product's settings sidebar say the same
// words, or the workbench describes a product that no longer exists.
//
// It did: 45 story files sat under `Settings/Admin settings/<old tab>/…`, the
// two-audience grouping this redesign deleted. A reader opening the workbench
// found "Admin settings" over a sidebar that has seven topic groups, and the
// old tab names underneath — a map of the previous product.
//
// The corpus is DERIVED from the tree rather than listed: a hand-written list
// of story files is a census that fails short, reporting PASS over a file it
// was never told about.

const SCREENS = new URL(".", import.meta.url).pathname;

function storyFilesUnder(dir: string): string[] {
  const found: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      found.push(...storyFilesUnder(path));
    } else if (entry.name.endsWith(".stories.tsx")) {
      found.push(path);
    }
  }
  return found;
}

// The group/page PAIRS the catalog declares, as the sidebar spells them. Pairs
// rather than two independent sets: `Settings/AI/Capture/Card` names a real
// group and a real page and is still wrong, because Capture belongs to Data.
// Two `has` checks cannot see that; one set of "Group/Page" strings can.
const PAGE_PATHS = new Set(
  SETTINGS_PAGES.map(
    (page) =>
      `${translate("en", `settings.group.${page.group}`)}/${translate(
        "en",
        `settings.tab.${page.id as SettingsPageId}`,
      )}`,
  ),
);

// The stories that are ABOUT a settings SURFACE rather than a card on a page,
// named exactly. Two segments each, because neither names a catalog page: the
// tree itself, and the settings home together with the boundary the same address
// answers when its segment names no page this reader can open (`SETTINGS_HOME_ID`
// is deliberately not a member of SETTINGS_PAGES). An
// `if (page === undefined) return` would exempt every two-segment title —
// `Settings/Nonsense` included — which is a skip-list with no list.
const SURFACE_STORIES = new Set([
  "Settings/Settings screen",
  "Settings/Settings home",
]);

// The one group that is not a catalog group. Two stories put cards from
// DIFFERENT pages side by side on purpose — the two price sheets an operator
// reconciles, and the two AI readings — so no single page name is true of
// either. Naming one would say the other card lives there.
//
// A declared exception with its members listed, not an open door: a third story
// cannot join by accident, and the day one of these stops crossing pages, its
// entry here fails rather than quietly staying.
const ACROSS_PAGES = "Across pages";
const ACROSS_PAGE_STORIES = new Set([
  "Settings/Across pages/Rates and model costs",
  "Settings/Across pages/AI readings",
]);

const settingsStories = storyFilesUnder(SCREENS)
  .map((path) => ({
    path: relative(SCREENS, path),
    title: storyTitle(path, readFileSync(path, "utf8")),
  }))
  .filter(
    (story): story is { path: string; title: string } =>
      story.title?.startsWith("Settings/") ?? false,
  );

// Every story file under screens/, whether or not it claims a Settings title.
// The corpus this gate must not lose a member of.
const everyStory = storyFilesUnder(SCREENS).map((path) => ({
  path: relative(SCREENS, path),
  title: storyTitle(path, readFileSync(path, "utf8")),
}));

describe("the settings stories are filed where the product files them", () => {
  // A floor is not a census. `>40` over 66 stories permits 25 to vanish — a
  // story whose title stops resolving, or whose root is edited away from
  // `Settings/`, drops out of the filtered corpus and is never checked again.
  // So the count is EXACT and derived from the tree: adding or removing a
  // settings story is a deliberate edit to this number.
  it("reads every settings story, and says how many that is", () => {
    expect(settingsStories.length).toBe(75);
  });

  // The filter above drops a file whose title does not resolve. That is the
  // silent direction: a story with a computed or missing title would leave the
  // corpus without failing anything. Every story file under screens/ must
  // therefore yield a title at all.
  it.each(everyStory)("resolves a title for $path", ({ title }) => {
    expect(title).not.toBeNull();
  });

  it.each(settingsStories)(
    "files $path at a path the catalog declares",
    ({ title }) => {
      if (SURFACE_STORIES.has(title)) {
        return;
      }
      const segments = title.split("/");
      if (segments[1] === ACROSS_PAGES) {
        // Named, so a new one is a deliberate edit here rather than a title
        // that quietly opts out of the check.
        expect(ACROSS_PAGE_STORIES).toContain(title);
        return;
      }
      // Exactly `Settings/<Group>/<Page>/<Card>`. A missing card segment and an
      // extra one both used to pass, because nothing read the length.
      expect(segments).toHaveLength(4);
      expect(PAGE_PATHS).toContain(`${segments[1]}/${segments[2]}`);
    },
  );
});

// WHAT THIS GATE DOES NOT HOLD, said plainly so the next author does not assume
// it does: that the page a story NAMES is the page whose dispatch arm renders
// its card. Two stories were misfiled that way once — the autonomy card sat
// under Agents while `tabContent` rendered it on Account, and mail sharing sat
// under Capture while it rendered on Connections. Both were found by hand, and
// both were resolved the other way in the end: the CARDS moved to the pages
// their stories had always named, because the stories were right about where
// each belonged.
//
// A gate for it was written and deleted. Reading the card-to-page map out of
// `settings.tsx` works for a card the screen renders directly, and stops at the
// ones nested inside another card (`VoiceCorpusIntake` inside `VoiceDnaCard`).
// Following the nesting one level then attributes a shared component to
// whichever card reached it first, which fails four correctly-filed stories.
// A gate that fails the honest cases teaches authors to file by whatever the
// gate accepts, which is worse than no gate.
//
// The real fix is for the screen to publish its card-to-page map as data rather
// than as a switch statement a test has to parse. That is a change to
// `settings.tsx`, not to this file.
