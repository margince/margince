import { expect, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * The address, and what Back does with it.
 *
 * A list's dials — its search, its order, its filters, its page size — used to
 * live in `useState`, so Back from a record arrived at an unfiltered list and a
 * link to "the list I am looking at" did not exist. These are the promises that
 * replaced that, driven the way a reader drives them: the browser's own Back
 * and Forward, never an in-app control that happens to go the same way.
 *
 * The mock serves one row per list whatever it is asked, so nothing here counts
 * rows. What is asserted instead is what the SERVER was asked for and what the
 * controls show — a narrowing the address claims but never sends would pass a
 * row count and still be broken.
 */

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

/** The `q` a request carries, or null for one that carries none. */
function searchOf(url: string): string | null {
  return new URL(url).searchParams.get("q");
}

const organizations = (url: string) =>
  url.includes("/v1/organizations") && !url.includes("/v1/organizations/");

test("a filtered list is what Back returns to", async ({ page }) => {
  await page.goto("/#/companies");
  await page.getByRole("searchbox", { name: "Suchen" }).fill("brandt");

  // The request is the proof the dial reached the server; the address is the
  // proof it can be returned to.
  await page.waitForRequest(
    (request) =>
      organizations(request.url()) && searchOf(request.url()) === "brandt",
  );
  await expect(page).toHaveURL(/[?&]q=brandt/);

  // Scoped to the page body: the rail now heads with the installation's own
  // company, and this fixture's company is also called Brandt, so an unscoped
  // /Brandt/ link would be the rail's home link rather than the row.
  await page
    .getByRole("main")
    .getByRole("link", { name: /Brandt/ })
    .first()
    .click();
  await expect(page).toHaveURL(/#\/companies\/o-brandt/);

  await page.goBack();

  // Both halves, because either alone is a broken screen: an address carrying
  // a filter over a list that never asked for one, or a filtered list at an
  // address nobody can share.
  await expect(page).toHaveURL(/[?&]q=brandt/);
  await expect(page.getByRole("searchbox", { name: "Suchen" })).toHaveValue(
    "brandt",
  );
});

test("a link opens the list it was copied from", async ({ page }) => {
  // Nobody typed here: this is somebody else's address, pasted cold.
  const asked = page.waitForRequest(
    (request) =>
      organizations(request.url()) && searchOf(request.url()) === "brandt",
  );
  await page.goto("/#/companies?q=brandt&sort=name");

  expect(new URL((await asked).url()).searchParams.get("sort")).toBe("name");
  await expect(page.getByRole("searchbox", { name: "Suchen" })).toHaveValue(
    "brandt",
  );
});

test("turning several dials does not bury the way out", async ({ page }) => {
  // Each dial REPLACES the entry rather than pushing one. Back is the one key
  // that exists for getting out of things, and a reader who narrowed a list
  // four ways must not have to press it five times to leave.
  await page.goto("/#/home");
  await page.goto("/#/companies");

  await page.getByRole("searchbox", { name: "Suchen" }).fill("brandt");
  await page.waitForRequest(
    (request) =>
      organizations(request.url()) && searchOf(request.url()) === "brandt",
  );
  await page.getByRole("searchbox", { name: "Suchen" }).fill("brandt gmbh");
  await page.waitForRequest(
    (request) =>
      organizations(request.url()) && searchOf(request.url()) === "brandt gmbh",
  );

  await page.goBack();
  await expect(page).toHaveURL(/#\/home$/);
});

test("Forward returns to the list Back left", async ({ page }) => {
  await page.goto("/#/companies");
  await page.getByRole("searchbox", { name: "Suchen" }).fill("brandt");
  await page.waitForRequest(
    (request) =>
      organizations(request.url()) && searchOf(request.url()) === "brandt",
  );

  await page
    .getByRole("main")
    .getByRole("link", { name: /Brandt/ })
    .first()
    .click();
  await expect(page).toHaveURL(/#\/companies\/o-brandt/);

  await page.goBack();
  await expect(page).toHaveURL(/[?&]q=brandt/);

  await page.goForward();
  await expect(page).toHaveURL(/#\/companies\/o-brandt/);
});

test("the address survives a reload, so a filtered list can be refreshed", async ({
  page,
}) => {
  await page.goto("/#/companies?q=brandt");
  await expect(page.getByRole("searchbox", { name: "Suchen" })).toHaveValue(
    "brandt",
  );

  await page.reload();

  await expect(page).toHaveURL(/[?&]q=brandt/);
  await expect(page.getByRole("searchbox", { name: "Suchen" })).toHaveValue(
    "brandt",
  );
});

test("an account's tab is an address, and Back steps between tabs", async ({
  page,
}) => {
  // The contact page has had this; the account page — the record a rep opens
  // most — kept its tab in `useState`, so a tab could not be linked to and
  // Back from one left the account altogether.
  await page.goto("/#/companies/o-brandt");
  await page.getByRole("button", { name: "Aufgaben", exact: true }).click();
  await expect(page).toHaveURL(/#\/companies\/o-brandt\/tasks$/);

  await page.getByRole("button", { name: "Verlauf", exact: true }).click();
  await expect(page).toHaveURL(/#\/companies\/o-brandt\/timeline$/);

  await page.goBack();
  await expect(page).toHaveURL(/#\/companies\/o-brandt\/tasks$/);
});

test("a linked tab opens on that tab, not on the overview", async ({
  page,
}) => {
  await page.goto("/#/companies/o-brandt/tasks");
  await expect(page.getByRole("button", { name: "Aufgaben" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
});

test("a section is an address, and Back steps between sections", async ({
  page,
}) => {
  await page.goto("/#/analytics");
  await page.getByRole("button", { name: "Pipeline" }).click();
  await expect(page).toHaveURL(/#\/analytics\/pipeline$/);

  await page.goBack();
  await expect(page).toHaveURL(/#\/analytics$/);
});

test("the deals board remembers its pipeline and its view in the address", async ({
  page,
}) => {
  // The deals screen hand-rolls its own list query because it drives a board as
  // well as a table, and none of it was addressable: the pipeline, the
  // board/table choice and every filter were lost on a reload.
  await page.goto("/#/deals?view=table&pipeline_id=pl-1");
  await expect(page).toHaveURL(/view=table/);
  await expect(page).toHaveURL(/pipeline_id=pl-1/);

  await page.reload();

  await expect(page).toHaveURL(/view=table/);
  await expect(page).toHaveURL(/pipeline_id=pl-1/);
});

test("paging is in the address, and Back returns to the page you left", async ({
  page,
}) => {
  // Page one is spelled by ABSENCE, so an address only carries a page once the
  // reader has moved off the first — a dial in front of somebody who turned
  // nothing is noise.
  await page.goto("/#/companies");
  await expect(page).not.toHaveURL(/[?&]page=/);

  await page.goto("/#/companies?page=2");
  await page
    .getByRole("main")
    .getByRole("link", { name: /Brandt/ })
    .first()
    .click();
  await expect(page).toHaveURL(/#\/companies\/o-brandt/);

  await page.goBack();
  await expect(page).toHaveURL(/[?&]page=2/);
});

test("a saved view moves the board to the pipeline it was saved on", async ({
  page,
}) => {
  // The pipeline is a DRAWING dial: held in the address, held out of the
  // request, and therefore carried across a list write rather than serialized
  // by the list codec. Carrying it unconditionally overwrote the pipeline a
  // saved view had just asked for, so the tab lit up while the board kept
  // showing the previous pipeline's stages.
  // The address already names a board — the state a reader is in the moment
  // they press a view's tab, and the value that used to win over the view.
  await page.goto("/#/deals?pipeline_id=pl");
  await expect(page.getByRole("region", { name: "Qualify" })).toBeVisible();

  await page.getByRole("button", { name: "Partner deals" }).click();

  // The address AND the board, because either alone is the defect: an address
  // naming the view's pipeline over stage columns from the other one is exactly
  // what a reader saw.
  await expect(page).toHaveURL(/pipeline_id=pl-partner/);
  await expect(page.getByRole("region", { name: "Referred" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Qualify" })).toHaveCount(0);
});

test("the leads board is an address", async ({ page }) => {
  await page.goto("/#/leads?view=board");
  await expect(page).toHaveURL(/view=board/);
  await page.reload();
  await expect(page).toHaveURL(/view=board/);
});
