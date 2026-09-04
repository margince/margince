import { expect, type Page, test } from "@playwright/test";

/**
 * The contact page against a LIVE stack, loaded in a real browser.
 *
 * This suite exists because the person page shipped a white "This view stopped
 * working" card and every unit test was green. The unit tests render the memory
 * card against fixtures the author wrote; nothing rendered the PAGE against the
 * payload the server actually sends, so a field the fixtures always included
 * and the server sometimes omits took the whole route down.
 *
 * So the rule this file holds is narrow and worth stating: the page LOADS, on
 * real data, with no uncaught error. Not what it looks like — the design suites
 * do that — but that a reader who clicks a contact sees the contact.
 *
 * It runs only with BASE_URL and a real contact id, and it fails loudly rather
 * than skipping when one is set without the other: a half-configured run that
 * quietly passes is the exact failure the suite was written to stop.
 */

const BASE_URL = process.env.BASE_URL;
const PERSON = process.env.E2E_PERSON;

if (BASE_URL && !PERSON) {
  throw new Error(
    "BASE_URL is set but E2E_PERSON is not — point it at a contact id on that stack",
  );
}

const live = test.describe[BASE_URL ? "serial" : "skip"];

async function signIn(page: Page) {
  await page.goto("/#/login", { waitUntil: "networkidle" });
  const email = page
    .locator('input[type="email"], input[name="email"]')
    .first();
  if ((await email.count()) === 0) {
    return;
  }
  await email.fill(process.env.E2E_EMAIL ?? "admin@demo.test");
  await page
    .locator('input[type="password"], input[name="password"]')
    .first()
    .fill(process.env.E2E_PASSWORD ?? "demo-password-123");
  await page.locator('button[type="submit"]').first().click();
  await expect(page.locator("nav.rail").first()).toBeVisible();
}

live("the contact page on live data", () => {
  test("loads without falling over", async ({ page }) => {
    // Every uncaught error, not only the one that happens to reach the DOM. A
    // boundary that catches a render crash draws a tidy card, so asserting on
    // what is visible alone can pass over a page that is entirely broken.
    const crashes: string[] = [];
    page.on("pageerror", (e) => crashes.push(e.stack ?? e.message));

    await signIn(page);
    await page.goto(`/#/contacts/${PERSON}`, { waitUntil: "networkidle" });

    // The error boundary's own words, in the three catalogs it can render in.
    // Asserting the boundary is ABSENT is the whole test: it is what the
    // reader saw instead of the page.
    await expect(
      page.getByText(/stopped working|nicht mehr|ngừng hoạt động/i),
    ).toHaveCount(0);
    expect(
      crashes,
      `uncaught error on the contact page:\n${crashes[0]}`,
    ).toEqual([]);

    // And something of the contact is actually drawn, so the assertion above
    // cannot pass over a blank page that merely failed quietly.
    await expect(page.locator("main")).not.toBeEmpty();
  });

  test("draws its retained email as the canonical row", async ({ page }) => {
    await signIn(page);
    await page.goto(`/#/contacts/${PERSON}`, { waitUntil: "networkidle" });

    // The row the whole unification arc is about. One is enough: this asserts
    // the canonical component is MOUNTED on real data, which is the half the
    // unit tests cannot see.
    await expect(page.locator(".emailentry").first()).toBeVisible();
  });

  // Opening the message, which is where it actually broke.
  //
  // The drawer crashed on every message with nobody in copy — the server sent
  // `cc: null` for a field the contract makes required, and the viewer read
  // `.length` off it. Every unit test passed: they assert Go structs and
  // hand-written fixtures, where nil and empty are indistinguishable and the
  // author always fills the field in.
  //
  // Asserting on `pageerror` alone does NOT catch this. React's error boundary
  // catches a render crash and draws a tidy card, so the page reports no
  // uncaught error while showing the reader nothing. The boundary's own words
  // are what has to be absent.
  test("opens the message in a drawer over the record", async ({ page }) => {
    await signIn(page);
    await page.goto(`/#/contacts/${PERSON}`, { waitUntil: "networkidle" });
    const address = page.url();

    // The OPENABLE row, which is a button; a row the reader may not open is a
    // div of the same class. Clicking `.emailentry` first-of-any picks
    // whichever came back first, and on a contact whose newest message is
    // withheld that is a div — the click does nothing, no drawer opens, no
    // crash happens, and the test passes having exercised none of this. It
    // did exactly that against a live null-cc build before this locator was
    // narrowed.
    const openable = page.locator("button.emailentry--open").first();
    await expect(openable).toBeVisible();
    await openable.click();

    await expect(page.getByRole("dialog")).toBeVisible();

    // The MESSAGE, not merely the dialog around it. The crash was inside
    // PartyLine, so the error boundary replaced the drawer's body while the
    // modal shell — title, close button — kept rendering perfectly: a test
    // that stopped at `dialog` is visible reported success over a reader
    // staring at "This view stopped working".
    //
    // The recipient line is the assertion because it is what broke: it is
    // drawn from the four party lists the server sends, and a null in any of
    // them takes it down.
    await expect(page.locator(".emaildetail__parties")).toBeVisible();
    await expect(
      page.getByText(
        /stopped working|funktioniert nicht mehr|ngừng hoạt động/i,
      ),
    ).toHaveCount(0);

    // A drawer OVER the record, not a page instead of it: the address does not
    // change and the shell stays drawn behind it. A modal that navigated would
    // lose the reader's place in the record they are working on.
    expect(page.url()).toBe(address);
    await expect(page.locator("nav.rail").first()).toBeVisible();
  });

  // EVERY tab, because the page is seven screens behind one address and a
  // reader reaches the other six by clicking. Testing only the one the URL
  // opens on is how a broken tab ships under a green suite.
  for (const tab of [
    "overview",
    "timeline",
    "network",
    "deals",
    "meetings",
    "research",
    "documents",
  ]) {
    test(`the ${tab} tab loads without falling over`, async ({ page }) => {
      const crashes: string[] = [];
      page.on("pageerror", (e) => crashes.push(e.stack ?? e.message));

      await signIn(page);
      await page.goto(`/#/contacts/${PERSON}/${tab}`, {
        waitUntil: "networkidle",
      });

      await expect(
        page.getByText(/stopped working|nicht mehr|ngừng hoạt động/i),
      ).toHaveCount(0);
      expect(
        crashes,
        `uncaught error on the ${tab} tab:\n${crashes[0]}`,
      ).toEqual([]);
    });
  }
});

/**
 * The other two surfaces the same arc changed, opened in a real browser for
 * the same reason: the account page's recent-exchange list and the held
 * threads table both draw a message, and the account's rows open the same
 * drawer.
 *
 * They are here rather than in files of their own because the claim is one
 * claim — a reader clicks a message and reads it — and the null that crashed
 * the contact page would have taken all three down together.
 */
live("the other surfaces that draw a message", () => {
  test("the account page loads and opens its messages", async ({ page }) => {
    test.skip(!process.env.E2E_ORG, "set E2E_ORG to an account on this stack");
    await signIn(page);
    await page.goto(`/#/companies/${process.env.E2E_ORG}`, {
      waitUntil: "networkidle",
    });
    await expect(
      page.getByText(/stopped working|nicht mehr|ngừng hoạt động/i),
    ).toHaveCount(0);

    const row = page.locator(".emailentry").first();
    // An account with no retained mail says nothing about the drawer, and
    // failing here would be reporting on the seed rather than on the code.
    if ((await row.count()) === 0) {
      return;
    }
    const address = page.url();
    await row.click();
    await expect(page.getByRole("dialog")).toBeVisible();
    await expect(
      page.getByText(/stopped working|nicht mehr|ngừng hoạt động/i),
    ).toHaveCount(0);
    expect(page.url()).toBe(address);
  });

  test("the held threads screen loads", async ({ page }) => {
    await signIn(page);
    await page.goto("/#/settings/held-threads", { waitUntil: "networkidle" });
    await expect(
      page.getByText(/stopped working|nicht mehr|ngừng hoạt động/i),
    ).toHaveCount(0);
  });
});
