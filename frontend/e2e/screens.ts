// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page } from "@playwright/test";
import { de } from "../src/i18n/de";
import type { MessageKey } from "../src/i18n/en";
import { SETTINGS_PAGES } from "../src/lib/settingscatalog";

/**
 * Every settings page, with no exclusion.
 *
 * `reset` stays in, though its page needs the deployment to arm
 * `data_reset_available`. `seed.ts` mocks `/me` to say it is armed. Rendering
 * the danger zone resets nothing, because the POST waits for a reader to type
 * the workspace name. It is the only settings page with an irreversible
 * action, so the sweeps and the heading check must read it.
 *
 * A page this principal cannot open is a bug in `E2E_ADMIN_GRANTS`.
 * `expectSettingsViewLanded` fails on it rather than shortening the census.
 */
const SWEPT_SETTINGS_PAGES = SETTINGS_PAGES.map((page) => page.id);

export const CORE_SCREENS = [
  "home",
  "contacts",
  "companies",
  "deals",
  // The lead queue has its own bulk-selection bar, board and toolbar. A reader
  // can swap its main view under the same address, so the 390px pass has two
  // layouts to check for sideways scroll.
  "leads",
  // Filters & views: its body is a nested editor of clause rows, an operator
  // control and a preview table. It is the screen most likely to grow wide:
  // a clause is a row of three controls, and each level of nesting indents it.
  "filters",
  "worklist",
  // Projects and Partners print the page's name inside their table header,
  // so the sweeps measure that header here.
  "projects",
  "partners",
  "analytics",
  "settings",
  // Settings is many pages behind one route (SETTINGS_PAGES in
  // lib/settingscatalog.ts), and a bare `settings` resolves to Account, the
  // shortest of them. The others are where a narrow viewport breaks. `fields`
  // carries two list surfaces with toolbars, `integrations` four cards,
  // `members` rows that each end in two buttons.
  //
  // The list is derived from the catalog, not written out. A hand-written list
  // can fall short, and a short census reports a pass on the smaller tree.
  //
  // `settingscatalog.ts` is pure data, so importing it pulls no screen module
  // through Playwright's transform. E2E_ADMIN_GRANTS carries every settings
  // object, so this principal opens every page. An ungranted page would land
  // on the access boundary and report a page the run never read.
  ...SWEPT_SETTINGS_PAGES.map((page) => `settings/${page}`),
];

/**
 * The settings page the address named is the one on screen.
 *
 * An address naming a page this mock's grants do not cover lands on the access
 * boundary, which renders cleanly. Axe and overflow then pass on a page the run
 * never meant to read, and the census counts one page too many.
 */
export async function expectSettingsViewLanded(page: Page, view: string) {
  if (!view.startsWith("settings/")) {
    return;
  }
  const tab = view.slice("settings/".length);
  // The page heading, because the nav level and the trail both leave a phone
  // bar. The heading is named by the tab at every width, its switcher included.
  // The expected wording comes from the catalog key the nav labels the tab with,
  // so this is not a second copy of the tab names.
  await expect(
    page.locator(".pagetitle").getByRole("heading"),
    `#/${view} did not land on the ${tab} tab — the address fell back, so this sweep measured a page it never opened`,
  ).toHaveAccessibleName(de[`settings.tab.${tab}` as MessageKey]);
}

/**
 * The page rendered, rather than the app error boundary standing in for it.
 *
 * Both sweeps measure what is on screen. A page that threw during render puts
 * almost nothing there, so it scores zero axe violations and zero overflow.
 * The error boundary is mounted above the rail (main.tsx), so a visible rail
 * proves the shell survived.
 */
export async function expectShellRendered(page: Page) {
  await expect(page.locator("nav.rail")).toBeVisible();
}
