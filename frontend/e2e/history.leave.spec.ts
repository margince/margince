// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page, type Request, test } from "@playwright/test";
import {
  attentionReport,
  HEALTH_AT,
} from "../src/features/settings/systemhealth.fixtures";
import { de } from "../src/i18n/de";
import { meFixture } from "../src/testing/mefixture";
import {
  reportingStoryEvaluation,
  reportingStoryReport,
} from "../src/testing/reporting.fixtures";
import { anna, brandt, E2E_ADMIN_GRANTS } from "./mockapi/fixtures";
import { company360, contact360 } from "./mockapi/records";
import { page as listPage } from "./mockapi/server";
import { mockApi, servePrincipal } from "./seed";

/**
 * Leaving a page, and coming back to it with the browser's own Back.
 *
 * A screen stays drawn for a moment after the reader leaves it, and an action
 * can finish after the reader has gone. Neither may write into the entry the
 * reader reached, and Back must find each entry as it was left. Asserted the
 * way `history.spec.ts` asserts it: what the server was asked and what the
 * controls show.
 */

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

const rail = (page: Page, name: string) =>
  page
    .getByRole("navigation", { name: de["shell.railAria"] })
    .getByRole("link", { name, exact: true });

const dialsOf = (url: string) =>
  new URLSearchParams(new URL(url).hash.split("?")[1] ?? "");

const contactsRead = (request: Request) =>
  /\/v1\/contacts(\?|$)/.test(request.url()) && request.method() === "GET";

test("the Overview's dials stay on the Overview when the reader leaves it", async ({
  page,
}) => {
  const asked: string[] = [];
  page.on("request", (request) => {
    if (contactsRead(request)) asked.push(request.url());
  });
  await page.goto(
    "/#/analytics/overview?overview=sales&period=last_quarter&scope=workspace",
  );
  await expect(
    page.getByRole("group", { name: de["analytics.filters"] }),
  ).toBeVisible();

  await rail(page, de["nav.contacts"]).click();
  await expect(page).toHaveURL(/#\/contacts(\?|$)/);
  await expect.poll(() => asked.length).toBeGreaterThan(0);

  const unturned = async () => {
    const dials = dialsOf(page.url());
    for (const dial of ["overview", "period", "scope"]) {
      expect(dials.has(dial)).toBe(false);
    }
    for (const url of asked) {
      const sent = new URL(url).searchParams;
      for (const dial of ["overview", "period", "scope"]) {
        expect(sent.has(dial)).toBe(false);
      }
    }
  };
  await unturned();

  await page.goBack();
  await expect(page).toHaveURL(/#\/analytics\/overview\?/);
  expect(dialsOf(page.url()).get("period")).toBe("last_quarter");

  await page.goForward();
  await expect(page).toHaveURL(/#\/contacts(\?|$)/);
  // The list drawn means the Overview is gone; a held one wrote a frame later.
  await expect(
    page.getByRole("main").getByRole("link", { name: /Anna Weber/ }),
  ).not.toHaveCount(0);
  await expect
    .poll(() =>
      ["overview", "period", "scope"].some((dial) =>
        dialsOf(page.url()).has(dial),
      ),
    )
    .toBe(false);
  await unturned();
});

// A regression guard: lists keep their page in memory, so this holds anyway.
test("Back from another list returns to the page of the list the reader left", async ({
  page,
}) => {
  const companies = Array.from({ length: 60 }, (_, at) => ({
    ...brandt,
    id: `o-filler-${at}`,
    name: `Filler ${at}`,
    display_name: `Filler ${at}`,
  }));
  await page.route(/\/v1\/companies(\?|$)/, (route) =>
    route.fulfill({ json: listPage(companies) }),
  );
  const pageThree = page
    .getByRole("navigation", { name: de["table.pagination"] })
    .getByRole("button", { name: de["table.page"].replace("{number}", "3") });
  await page.goto("/#/companies?page=3");
  await expect(pageThree).toHaveAttribute("aria-current", "page");

  await rail(page, de["nav.contacts"]).click();
  await expect(page).toHaveURL(/#\/contacts(\?|$)/);

  await page.goBack();
  await expect(page).toHaveURL(/#\/companies\?page=3$/);
  await expect(pageThree).toHaveAttribute("aria-current", "page");
  await expect(
    page.getByRole("main").getByText("Filler 50", { exact: true }),
  ).toBeVisible();
});

// The address has depth and a query, so a Stay that restores only part of it
// shows.
const retentionForm =
  "/#/settings/system-health/configuration?section=operations.retention.label";

async function openRetentionForm(page: Page) {
  await servePrincipal(
    page,
    meFixture({
      allow: {
        job_health: ["read"],
        installation_settings: ["read", "update"],
        // A test that leaves the form for the companies list needs the list
        // drawn; a role without this read gets a refusal page instead.
        company: ["read"],
      },
    }),
  );
  await page.goto("/#/home");
  await expect(page.getByRole("main")).toBeVisible();
  await page.goto(retentionForm);
  const field = page.getByTestId("operation-retention_sweep_interval_seconds");
  await expect(field).toBeFocused();
  return field;
}

const answerStay = (page: Page) =>
  page
    .getByRole("dialog")
    .getByRole("button", { name: de["unsaved.stay"], exact: true })
    .click();

test("Stay after a link puts the whole address back, and the declined page still asks", async ({
  page,
}) => {
  const field = await openRetentionForm(page);
  const address = page.url();
  await field.fill("7200");
  await field.press("Tab");

  // While settings is open the rail holds the settings entries.
  await page
    .getByRole("navigation", { name: de["shell.railAria"] })
    .locator('a[href="/#/settings/account"]')
    .click();
  await answerStay(page);

  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(page).toHaveURL(address);
  await expect(field).toHaveValue("7200");

  // History cannot drop the entry ahead, so the declined page stays one
  // Forward away.
  await page.goForward();
  await expect(page).toHaveURL(/#\/settings\/account$/);
  await answerStay(page);
  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(page).toHaveURL(address);
  await expect(field).toHaveValue("7200");

  // The draft is still unsaved, so leaving by Back asks too.
  await page.goBack();
  await expect(page).toHaveURL(/#\/home$/);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: de["saveBar.discard"], exact: true })
    .click();
  await expect(field).toBeHidden();
});

test("Stay pressed twice while its way back is under way takes one step", async ({
  page,
}) => {
  const field = await openRetentionForm(page);
  const address = page.url();
  await field.fill("7200");
  await field.press("Tab");
  await page
    .getByRole("navigation", { name: de["shell.railAria"] })
    .locator('a[href="/#/settings/account"]')
    .click();
  const entries = await page.evaluate(() => history.length);

  const stay = page
    .getByRole("dialog")
    .getByRole("button", { name: de["unsaved.stay"], exact: true });
  const box = await stay.boundingBox();
  if (box === null) throw new Error("the Stay button is not drawn");
  await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);

  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(page).toHaveURL(address);
  expect(await page.evaluate(() => history.length)).toBe(entries);
  await expect(field).toHaveValue("7200");

  await page.goBack();
  await expect(page).toHaveURL(/#\/home$/);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: de["saveBar.discard"], exact: true })
    .click();
  await expect(field).toBeHidden();
});

test("Stay after Back returns to the form and keeps the way Forward", async ({
  page,
}) => {
  const field = await openRetentionForm(page);
  const address = page.url();
  await page.goto("/#/companies");
  // The list spells its opening sort into its own entry once it mounts.
  await expect(page).toHaveURL(/#\/companies\?sort=-created_at$/);
  const list = page.url();
  await page.goBack();
  await expect(page).toHaveURL(address);
  await field.fill("7200");
  await field.press("Tab");

  await page.goBack();
  await answerStay(page);

  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(page).toHaveURL(address);
  await expect(field).toHaveValue("7200");

  await page.goForward();
  await expect(page).toHaveURL(list);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: de["saveBar.discard"], exact: true })
    .click();
  await expect(field).toBeHidden();
});

const topBarBack = (page: Page) =>
  // The top bar sits inside main, so it is no banner landmark.
  page.locator("header.topbar").getByRole("button", {
    name: de["shell.back"],
    exact: true,
  });

// A deal keeps its tab on the page; an account's tab is an entry of its own,
// which the arrow must step over.
const records = [
  {
    kind: "a deal",
    list: de["nav.deals"],
    row: /Fleet retrofit/,
    at: /#\/deals\/d-fleet$/,
    tab: de["tab.history"],
    listed: /#\/deals(\?[^/]*)?$/,
  },
  {
    kind: "an account",
    list: de["nav.companies"],
    row: /Brandt/,
    at: /#\/companies\/o-brandt$/,
    tab: de["tab.timeline"],
    listed: /#\/companies(\?[^/]*)?$/,
  },
] as const;

type RecordWalk = (typeof records)[number];

// Home, the list, one record, then its history tab. A rep walks this before
// reaching for the arrow.
async function openRecordHistory(page: Page, record: RecordWalk) {
  await page.goto("/#/home");
  await expect(page.getByRole("main")).toBeVisible();
  await rail(page, record.list).click();
  await expect(page).toHaveURL(record.listed);
  await page
    .getByRole("main")
    .getByRole("link", { name: record.row })
    .first()
    .click();
  await expect(page).toHaveURL(record.at);
  await page
    .getByRole("main")
    .getByRole("button", { name: record.tab, exact: true })
    .click();
  await expect(
    page
      .getByRole("main")
      .getByRole("button", { name: record.tab, exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
}

for (const record of records) {
  test(`a double press on ${record.kind}'s Back arrow takes one step out`, async ({
    page,
  }) => {
    await openRecordHistory(page, record);
    const box = await topBarBack(page).boundingBox();
    if (box === null) throw new Error("the Back arrow is not drawn");
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    await page.mouse.click(x, y);
    await page.mouse.click(x, y);

    await expect(page).toHaveURL(record.listed);
    await page.goBack();
    await expect(page).toHaveURL(/#\/home$/);
  });

  test(`a double-click on ${record.kind}'s Back arrow takes one step out`, async ({
    page,
  }) => {
    await openRecordHistory(page, record);
    await topBarBack(page).dblclick();

    await expect(page).toHaveURL(record.listed);
    await page.goBack();
    await expect(page).toHaveURL(/#\/home$/);
  });

  test(`${record.kind}'s Back arrow still finds the list after a reload`, async ({
    page,
  }) => {
    await openRecordHistory(page, record);
    await page.reload();
    await expect(topBarBack(page)).toBeVisible();

    await topBarBack(page).click();

    await expect(page).toHaveURL(record.listed);
    await expect(
      page.getByRole("main").getByRole("link", { name: record.row }),
    ).not.toHaveCount(0);
  });
}

const queueToggle = (page: Page, label: "show" | "hide") =>
  // The name ends in the queue's count, which the fixture is free to change.
  page.getByRole("button", { name: de[`brief.queue.${label}`] });

const queueDrawer = (page: Page) =>
  page.getByRole("dialog", { name: de["brief.queue.title"] });

async function openHomeFromCompanies(page: Page) {
  await page.goto("/#/companies");
  await expect(page.getByRole("main")).toBeVisible();
  await rail(page, de["nav.brief"]).click();
  await expect(page).toHaveURL(/#\/home$/);
  await expect(queueToggle(page, "show")).toBeVisible();
}

test("a Worklist drawer closed from itself leaves no entry that reopens it", async ({
  page,
}) => {
  await openHomeFromCompanies(page);
  await queueToggle(page, "show").click();
  await expect(queueDrawer(page)).toBeVisible();
  await expect(page).toHaveURL(/[?&]queue=1/);
  // Hide sits under the open drawer, so its own close is the way a pointer
  // closes it; both answer through one close.
  await expect(queueToggle(page, "hide")).toHaveAttribute(
    "aria-pressed",
    "true",
  );

  await queueDrawer(page)
    .getByRole("button", { name: de["common.close"], exact: true })
    .click();
  await expect(queueDrawer(page)).toBeHidden();
  await expect(page).toHaveURL(/#\/home$/);

  await page.goBack();
  await expect(page).toHaveURL(/#\/companies(\?|$)/);
  await expect(queueDrawer(page)).toBeHidden();
});

test("Back closes the Worklist drawer onto Home", async ({ page }) => {
  await openHomeFromCompanies(page);
  await queueToggle(page, "show").click();
  await expect(queueDrawer(page)).toBeVisible();

  await page.goBack();
  await expect(queueDrawer(page)).toBeHidden();
  await expect(page).toHaveURL(/#\/home$/);
});

test("a double press on Show opens the Worklist drawer on one entry", async ({
  page,
}) => {
  await openHomeFromCompanies(page);
  const entries = await page.evaluate(() => history.length);
  const box = await queueToggle(page, "show").boundingBox();
  if (box === null) throw new Error("the Worklist toggle is not drawn");
  await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);

  await expect(queueDrawer(page)).toBeVisible();
  expect(await page.evaluate(() => history.length)).toBe(entries + 1);
  await page.goBack();
  await expect(queueDrawer(page)).toBeHidden();
  await expect(page).toHaveURL(/#\/home$/);
});

async function dealsFromContacts(page: Page) {
  await page.goto("/#/contacts");
  await expect(page.getByRole("main")).toBeVisible();
  await rail(page, de["nav.deals"]).click();
  await expect(page).toHaveURL(/#\/deals(\?|$)/);
  await expect(
    page.getByRole("main").getByRole("link", { name: /Fleet retrofit/ }),
  ).not.toHaveCount(0);
}

test("Back closes the command palette with the page it opened on", async ({
  page,
}) => {
  await dealsFromContacts(page);
  const palette = page.getByRole("dialog", { name: de["palette.aria"] });
  await page.keyboard.press("ControlOrMeta+k");
  await expect(palette).toBeVisible();

  await page.goBack();

  await expect(page).toHaveURL(/#\/contacts(\?|$)/);
  await expect(palette).toBeHidden();
  await expect(
    page.getByRole("main").getByRole("link", { name: /Anna Weber/ }),
  ).not.toHaveCount(0);
});

test("Back closes a dialog held in state with the page it opened on", async ({
  page,
}) => {
  await dealsFromContacts(page);
  const shortcuts = page.getByRole("dialog", {
    name: de["shell.help.shortcuts"],
  });
  await page.keyboard.press("?");
  await expect(shortcuts).toBeVisible();

  await page.goBack();

  await expect(page).toHaveURL(/#\/contacts(\?|$)/);
  await expect(shortcuts).toBeHidden();
});

test("a drawer that turns a dial of its own page stays open", async ({
  page,
}) => {
  await openHomeFromCompanies(page);
  await queueToggle(page, "show").click();
  await expect(queueDrawer(page)).toBeVisible();

  await queueDrawer(page)
    .getByRole("button", { name: de["worklist.view.decisions"] })
    .click();

  await expect(page).toHaveURL(/[?&]queue_view=decisions/);
  await expect(queueDrawer(page)).toBeVisible();
});

test("a Qualify dialog the reader closed stays closed on Back", async ({
  page,
}) => {
  await page.goto("/#/leads/l-1?action=qualify");
  const qualify = page.getByRole("dialog");
  await expect(qualify.getByText(de["lead.previewCreate"])).toBeVisible();

  await page.keyboard.press("Escape");
  await expect(qualify).toBeHidden();
  await expect(page).toHaveURL(/#\/leads\/l-1$/);

  await rail(page, de["nav.deals"]).click();
  await expect(page).toHaveURL(/#\/deals(\?|$)/);
  await page.goBack();

  await expect(page).toHaveURL(/#\/leads\/l-1$/);
  await expect(page.getByTestId("lead-qualify")).toBeVisible();
  await expect(qualify).toBeHidden();
});

// A principal who may archive, with the delete grant the shared admin lacks.
async function serveArchiver(page: Page, object: "contact" | "company") {
  const admin = meFixture({
    allow: { ...E2E_ADMIN_GRANTS, [object]: ["read", "update", "delete"] },
  });
  await servePrincipal(page, {
    ...admin,
    user: { ...admin.user, id: "u1", locale: "de" },
  });
}

// The record's own read and its 360, archivable, and a DELETE that archives.
async function serveArchivable(
  page: Page,
  record: string,
  readable: object,
  overview: object,
) {
  await page.route(`**/v1/${record}/360`, (route) =>
    route.fulfill({ json: overview }),
  );
  await page.route(`**/v1/${record}`, (route) =>
    route.fulfill({
      json:
        route.request().method() === "DELETE"
          ? { ...readable, archived_at: "2026-11-16T09:00:00Z" }
          : readable,
    }),
  );
}

async function archiveFromHeader(page: Page) {
  await page
    .getByRole("main")
    .getByRole("button", { name: de["record.moreActions"] })
    .first()
    .click();
  await page.getByRole("button", { name: de["record.archive"] }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: de["record.archive"], exact: true })
    .click();
}

test("archiving a contact goes back to the filtered list it came from", async ({
  page,
}) => {
  await serveArchiver(page, "contact");
  const archivable = { ...anna, archivable: true };
  await serveArchivable(page, "contacts/p-anna", archivable, {
    ...contact360,
    contact: archivable,
  });
  await page.goto("/#/home");
  await expect(page.getByRole("main")).toBeVisible();
  await page.goto("/#/contacts?owner=me");
  const row = page.getByRole("main").getByRole("link", { name: /Anna Weber/ });
  await expect(row).not.toHaveCount(0);
  const list = page.url();
  expect(dialsOf(list).get("owner")).toBe("me");
  await row.first().click();
  await expect(page).toHaveURL(/#\/contacts\/p-anna$/);
  const record = page.url();

  await archiveFromHeader(page);

  await expect(page).toHaveURL(list);
  await page.goBack();
  await expect(page).toHaveURL(/#\/home$/);
  await page.goForward();
  await expect(page).toHaveURL(list);
  // The archived record stays one Forward away; history cannot drop it.
  await page.goForward();
  await expect(page).toHaveURL(record);
});

test("archiving an account from one of its tabs goes back past the tabs to the list", async ({
  page,
}) => {
  await serveArchiver(page, "company");
  const archivable = { ...brandt, archivable: true };
  await serveArchivable(page, "companies/o-brandt", archivable, {
    ...company360,
    company: archivable,
  });
  await page.goto("/#/home");
  await expect(page.getByRole("main")).toBeVisible();
  await rail(page, de["nav.companies"]).click();
  const row = page.getByRole("main").getByRole("link", { name: /Brandt/ });
  await expect(row).not.toHaveCount(0);
  const list = page.url();
  await row.first().click();
  await expect(page).toHaveURL(/#\/companies\/o-brandt$/);
  await page
    .getByRole("main")
    .getByRole("button", { name: de["tab.timeline"], exact: true })
    .click();
  await expect(page).toHaveURL(/#\/companies\/o-brandt\/timeline$/);

  await archiveFromHeader(page);

  await expect(page).toHaveURL(list);
  await page.goBack();
  await expect(page).toHaveURL(/#\/home$/);
  await page.goForward();
  await expect(page).toHaveURL(list);
  await page.goForward();
  await expect(page).toHaveURL(/#\/companies\/o-brandt$/);
});

// Holds the deal create until the test lets it answer, so the reader can act
// while it is in flight.
async function holdDealCreate(page: Page) {
  let release = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/v1/deals", async (route) => {
    if (route.request().method() === "POST") await held;
    await route.fallback();
  });
  return release;
}

async function submitNewDeal(page: Page) {
  await page.getByRole("button", { name: de["create.deal"] }).first().click();
  await page.getByLabel(de["create.dealName"]).fill("Neuer Deal");
  const posted = page.waitForRequest(
    (request) =>
      request.method() === "POST" && request.url().endsWith("/v1/deals"),
  );
  await page.getByRole("button", { name: "Erstellen", exact: true }).click();
  await posted;
}

// The rail's line for a write in flight, gone once the create has settled.
const creatingDeal = (page: Page) =>
  page
    .locator(".arblock")
    .getByText(de["agent.ticker.write.dealNew"], { exact: true });

test("a create that answers after the reader left does not pull them back", async ({
  page,
}) => {
  const release = await holdDealCreate(page);
  await page.goto("/#/contacts");
  await expect(
    page.getByRole("main").getByRole("link", { name: /Anna Weber/ }),
  ).not.toHaveCount(0);
  await page.goto("/#/deals");
  await submitNewDeal(page);

  await page.goBack();
  await expect(page).toHaveURL(/#\/contacts(\?|$)/);
  await expect(creatingDeal(page)).toBeVisible();
  release();

  await expect(creatingDeal(page)).toBeHidden();
  await expect(page).toHaveURL(/#\/contacts(\?|$)/);
});

test("a search typed during a create is not leaving, so the new deal opens", async ({
  page,
}) => {
  const release = await holdDealCreate(page);
  await page.goto("/#/deals");
  await submitNewDeal(page);

  // The box sits under the open form: fill reaches it where a pointer cannot.
  await page
    .getByRole("main")
    .getByRole("combobox", { name: "Suchen" })
    .fill("brandt");
  await expect(page).toHaveURL(/#\/deals\?(.*&)?q=brandt/);
  release();

  await expect(page).toHaveURL(/#\/deals\/d-new$/);
});

async function serveJobHistory(page: Page) {
  await servePrincipal(
    page,
    meFixture({
      allow: { job_health: ["read"], installation_settings: ["read"] },
    }),
  );
  await page.route("**/v1/admin/system-health", (route) =>
    route.fulfill({ json: attentionReport }),
  );
  await page.route("**/v1/admin/job-health", (route) =>
    route.fulfill({
      json: {
        generated_at: HEALTH_AT,
        dead_window_hours: 24,
        kinds: [],
        recent_failures: [],
      },
    }),
  );
  await page.route("**/v1/admin/job-health/history*", (route) =>
    route.fulfill({
      json: {
        observed_at: HEALTH_AT,
        cutoff: HEALTH_AT,
        window_start: HEALTH_AT,
        groups: [],
        runs: [],
        has_more: false,
        population_may_have_changed: false,
        coverage: "retained_only",
      },
    }),
  );
}

test("the job history's state pill follows its address across Back and Forward", async ({
  page,
}) => {
  await serveJobHistory(page);
  const historyAsked: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/v1/admin/job-health/history")) {
      historyAsked.push(new URL(request.url()).searchParams.get("state") ?? "");
    }
  });
  await page.goto("/#/settings/system-health");
  await expect(page.getByRole("main")).toBeVisible();
  await page.goto(
    "/#/settings/system-health/diagnostics?section=settings.jobs",
  );
  const states = page.getByRole("group", { name: de["jobs.col.state"] });
  const pill = (key: "retryable" | "cancelled") =>
    states.getByRole("button", { name: de[`jobs.state.${key}`] });

  await pill("retryable").click();
  await expect(pill("retryable")).toHaveAttribute("aria-pressed", "true");
  await expect(pill("retryable")).toBeFocused();
  await expect(page).toHaveURL(/[?&]state=retryable/);

  await pill("cancelled").click();
  await expect(pill("cancelled")).toHaveAttribute("aria-pressed", "true");
  await expect(pill("cancelled")).toBeFocused();
  await expect(page).toHaveURL(/[?&]state=cancelled/);
  await expect.poll(() => historyAsked.at(-1)).toBe("cancelled");

  await page.goBack();
  await expect(page).toHaveURL(/#\/settings\/system-health$/);
  await expect(states).toBeHidden();

  await page.goForward();
  await expect(page).toHaveURL(/[?&]state=cancelled/);
  await expect(pill("cancelled")).toHaveAttribute("aria-pressed", "true");
  await expect(pill("retryable")).toHaveAttribute("aria-pressed", "false");

  // An entry of its own on the same screen, which stays drawn across Back.
  await page.goto(
    "/#/settings/system-health/diagnostics?section=settings.jobs&state=retryable",
  );
  await expect(pill("retryable")).toHaveAttribute("aria-pressed", "true");
  await page.goBack();
  await expect(page).toHaveURL(/[?&]state=cancelled/);
  await expect(pill("cancelled")).toHaveAttribute("aria-pressed", "true");
  await expect(pill("retryable")).toHaveAttribute("aria-pressed", "false");
  await page.goForward();
  await expect(pill("retryable")).toHaveAttribute("aria-pressed", "true");
  await expect(pill("cancelled")).toHaveAttribute("aria-pressed", "false");
});

test("Back from a saved custom report returns to Reports, not its preview", async ({
  page,
}) => {
  const admin = meFixture({
    allow: {
      ...E2E_ADMIN_GRANTS,
      report_definition: ["read", "create", "update"],
    },
  });
  await servePrincipal(page, {
    ...admin,
    user: { ...admin.user, id: "u1", locale: "de" },
  });
  const saved = { ...reportingStoryReport, id: "r-saved", name: "Pipeline" };
  const posts: string[] = [];
  await page.route("**/v1/analytics/reports", (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    posts.push(route.request().url());
    return route.fulfill({ status: 201, json: saved });
  });
  await page.route("**/v1/analytics/reports/r-saved", (route) =>
    route.fulfill({ json: saved }),
  );
  await page.route("**/v1/analytics/reports/r-saved/evaluation", (route) =>
    route.fulfill({ json: reportingStoryEvaluation }),
  );

  await page.goto("/#/analytics/reports");
  await page
    .getByRole("button", { name: de["reporting.newCustomReport"] })
    .click();
  await page.getByLabel(de["reporting.name"]).fill("Pipeline");
  await page
    .getByRole("button", { name: de["reporting.previewReport"] })
    .click();
  await expect(page).toHaveURL(/#\/analytics\/reports\/preview/);

  await page.getByRole("button", { name: de["reporting.save"] }).click();
  await expect(page).toHaveURL(/#\/analytics\/reports\/r-saved/);
  expect(posts).toHaveLength(1);
  await expect(
    page.getByRole("main").getByRole("heading", { name: saved.name }).first(),
  ).toBeVisible();

  await page.goBack();
  await expect(page).toHaveURL(/#\/analytics\/reports(\?[^/]*)?$/);
  await expect(
    page.getByRole("button", { name: de["reporting.newCustomReport"] }),
  ).toBeVisible();
  expect(posts).toHaveLength(1);
});

// Holds every request to `url` until the returned function is called, then
// answers with `json`, or passes it on when there is none.
async function holdRoute(page: Page, url: string, json?: object) {
  let release = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(url, async (route) => {
    await held;
    await (json === undefined ? route.fallback() : route.fulfill({ json }));
  });
  return release;
}

test("an archived account's late redirect stays off the page the reader moved to", async ({
  page,
}) => {
  const gone = {
    ...brandt,
    id: "o-gone",
    display_name: "Stillgelegt GmbH",
    archived_at: "2026-11-01T09:00:00Z",
  };
  const releaseRead = await holdRoute(page, "**/v1/companies/o-gone", gone);
  await page.route("**/v1/companies/o-gone/360", (route) =>
    route.fulfill({ json: { ...company360, company: gone } }),
  );
  await page.goto("/#/home");
  await expect(page.getByRole("main")).toBeVisible();
  const asked = page.waitForRequest("**/v1/companies/o-gone");
  // An archived account refuses its deals tab, so this address redirects.
  await page.goto("/#/companies/o-gone/deals");
  await asked;

  // Home's code ships in the first load, so Back to it holds nothing. A screen
  // loaded on demand keeps the account drawn past the move while it arrives.
  const releaseCode = await holdRoute(page, "**/assets/**");
  await rail(page, de["nav.salesGoals"]).click();
  await expect(page).toHaveURL(/#\/goals$/);

  releaseRead();
  const held = page.getByRole("main").getByText(gone.display_name);
  await expect(held.first()).toBeVisible();
  await expect(page).toHaveURL(/#\/goals$/);
  releaseCode();

  await expect(held).toHaveCount(0);
  await expect(page).toHaveURL(/#\/goals$/);
});
