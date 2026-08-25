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

  await page
    .getByRole("link", { name: /Brandt/ })
    .first()
    .click();
  await expect(page).toHaveURL(/#\/companies\/o-brandt/);

  await page.goBack();

  // Both halves, because either alone is a broken screen: an address carrying
  // a filter over a list that never asked for one, or a filtered list at an
  // address nobody can share.
  await expect(page).toHaveURL(/[?&]q=brandt/);
  await expect(page.getByRole("searchbox", { name: "Suchen" })).toHaveValue("brandt");
});

test("a link opens the list it was copied from", async ({ page }) => {
  // Nobody typed here: this is somebody else's address, pasted cold.
  const asked = page.waitForRequest(
    (request) =>
      organizations(request.url()) && searchOf(request.url()) === "brandt",
  );
  await page.goto("/#/companies?q=brandt&sort=name");

  expect(new URL((await asked).url()).searchParams.get("sort")).toBe("name");
  await expect(page.getByRole("searchbox", { name: "Suchen" })).toHaveValue("brandt");
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
  await expect(page.getByRole("searchbox", { name: "Suchen" })).toHaveValue("brandt");

  await page.reload();

  await expect(page).toHaveURL(/[?&]q=brandt/);
  await expect(page.getByRole("searchbox", { name: "Suchen" })).toHaveValue("brandt");
});
