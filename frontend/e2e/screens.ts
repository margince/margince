// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page } from "@playwright/test";
import { de } from "../src/i18n/de";
import type { MessageKey } from "../src/i18n/en";
import { SETTINGS_PAGES } from "../src/lib/settingscatalog";

/**
 * Every settings page, with no exclusion.
 *
 * There was one — `reset`, whose page needs the deployment to arm
 * `data_reset_available`. That is a fixture fact rather than a product one:
 * `seed.ts` mocks `/me` and can say the deployment arms it, and RENDERING the
 * danger zone resets nothing (the POST waits for a reader to type the workspace
 * name). Excluding it left the one settings page carrying an irreversible verb
 * out of the 390px sweep, both axe passes and the heading check.
 *
 * A page this principal cannot open is now a BUG in the grants above, and
 * `expectSettingsViewLanded` turns it into a failure rather than a silently
 * shorter census.
 */
const SWEPT_SETTINGS_PAGES = SETTINGS_PAGES.map((page) => page.id);

export const CORE_SCREENS = [
  "home",
  "contacts",
  "companies",
  "deals",
  // The lead queue. It was in neither sweep while it grew a bulk-selection
  // bar, a board and a toolbar of its own — and it is the one record list
  // whose primary surface a reader can swap under the same address, so the
  // 390px pass has two layouts to find a horizontal scroll in rather than one.
  "leads",
  // Filters & views. It is a rail destination AC-shell-1 already asserts by
  // name, and it was in neither sweep — so the one screen in the product whose
  // body is a nested editor grew clause rows, an operator control and a preview
  // table with nobody measuring them at 390px or against axe. It is also the
  // screen most able to be WIDE: a clause is a row of three controls, and a
  // second level of nesting indents it again.
  "filters",
  "worklist",
  // Projects and Partners. Both are rail-or-off-rail list screens that now
  // print the page's name inside their table header, and both were in neither
  // sweep — so the two screens where that header is NOT measured were the two
  // whose header this change rewrote. The sweep follows the surface.
  "projects",
  "partners",
  "analytics",
  "settings",
  // Settings is many pages behind one route (SETTINGS_PAGES in
  // lib/settingscatalog.ts), and a bare `settings` resolves to Account,
  // the shortest of them. Sweeping that alone and calling settings covered is
  // how the widest page in the product went unmeasured: `fields` carries two
  // full list surfaces with their own toolbars, `integrations` four
  // installation-wide cards, `members` a roster of rows that each end in two
  // buttons. These are where a narrow viewport actually breaks.
  //
  // DERIVED from the catalog, not written out. The list that stood here went
  // stale twice — once claiming twelve while three pages were in neither sweep,
  // and again when the catalog was split and six of its ids were renamed under
  // it. A census that can fall short reports PASS on the smaller tree, which is
  // the one failure mode a sweep must not have.
  //
  // The blocker that kept it hand-written is gone in both halves.
  // `settingscatalog.ts` is pure data and imports like `../src/i18n/de` above,
  // so reading the ids pulls no screen module through Playwright's transform.
  // And E2E_ADMIN_GRANTS now carries the thirteen settings objects, so this
  // principal opens every page named below — without that, each ungranted page
  // would land on the access boundary and report a page the run never read.
  ...SWEPT_SETTINGS_PAGES.map((page) => `settings/${page}`),
];

/**
 * The settings page the address named is the one on screen.
 *
 * An address naming a page this mock's grants do not cover lands on the access
 * boundary and renders perfectly — clean axe, no overflow, and the census one
 * page longer than the tree it actually read. Two of the three pages this list
 * gained were doing exactly that. The active row in the settings level carries
 * the tab it points at, which is the cheapest thing on screen that can tell the
 * two apart.
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
 * Both sweeps below measure what is on screen, and a page that threw during
 * render puts almost nothing there: `settings/maintenance` crashed on a
 * malformed `/admin/job-health` payload and scored zero axe violations and zero
 * overflow — a dead page passing every check. The rail is the cheapest proof
 * the shell survived, because the boundary is mounted ABOVE it (main.tsx): if
 * the fallback is showing, the rail is gone.
 */
export async function expectShellRendered(page: Page) {
  await expect(page.locator("nav.rail")).toBeVisible();
}
