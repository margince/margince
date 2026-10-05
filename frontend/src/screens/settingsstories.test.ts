// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { storyCensus } from "../../scripts/lib/story-files";
import { titledStories } from "../../scripts/lib/story-title";
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

const frontendRoot = resolve(__dirname, "..", "..");
const srcDir = join(frontendRoot, "src");

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
// named exactly. Two segments each, because none names a catalog page: the
// tree itself, the sidebar's settings level, and the settings home together with
// the boundary the same address answers when its segment names no page this
// reader can open (`SETTINGS_HOME_ID` is deliberately not a member of
// SETTINGS_PAGES). An
// `if (page === undefined) return` would exempt every two-segment title —
// `Settings/Nonsense` included — which is a skip-list with no list.
const SURFACE_STORIES = new Set([
  "Settings/Settings screen",
  "Settings/Settings navigation",
  "Settings/Settings home",
]);

// The one group that is not a catalog group, and there are two ways into it.
//
// A story can put cards from DIFFERENT pages side by side on purpose — the two
// price sheets an operator reconciles, and the two AI readings — so that no
// single page name is true of it. Or its SUBJECT can be one control that two
// pages mount: the refresh verb rides the currency sheet and the model lanes
// both, and filing it under either would tell a reader the other page does not
// offer it.
//
// A declared exception with its members listed, not an open door: a fourth
// story cannot join by accident, and the day one of these stops crossing pages,
// its entry here fails rather than quietly staying.
const ACROSS_PAGES = "Across pages";
const ACROSS_PAGE_STORIES = new Set([
  "Settings/Across pages/Rates and model costs",
  "Settings/Across pages/AI readings",
  "Settings/Across pages/Refresh from sources",
  // The units an installation composed: the manifest's declared secret scope
  // decides whether a unit is offered on a member's own Connections page or
  // under Integrations, so the card is one subject that both pages mount and
  // filing it under either would say the other page does not offer it.
  "Settings/Across pages/Units offered in settings",
]);

// Every story file Storybook loads: a settings card's story can sit beside a
// component outside screens/, as mail-history's does.
const settingsStories = (await titledStories(storyCensus(frontendRoot).files))
  .map(({ path, title }) => ({ path: relative(srcDir, path), title }))
  .filter(
    (story): story is { path: string; title: string } =>
      story.title?.startsWith("Settings/") ?? false,
  );

describe("the settings stories are filed where the product files them", () => {
  // Pinned by hand rather than floored: a story retitled away from `Settings/`
  // drops out of the corpus above, and only an exact count notices it go.
  it("reads every settings story, and says how many that is", () => {
    expect(settingsStories.length).toBe(122);
  });

  // A file whose title does not resolve drops out of the filter above;
  // catalog.test.ts fails every such file under src/.

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
