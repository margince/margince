import { expect, type Locator, type Page, test } from "@playwright/test";
import { signIn } from "./waits";

/**
 * The company record page against the design's glance (DESIGN.md §7).
 *
 * This suite exists because "it looks like the mockup" kept being reported from
 * reading the code rather than the page. TWO describes, and the split matters:
 *
 *   - page shape — which regions exist and in what order;
 *   - visual weight — how big and how prominent they are.
 *
 * Shape alone is not the requirement, and asserting only shape is how a page
 * with the right skeleton at half the mockup's scale passed every check while
 * looking nothing like it. The second describe measures what is drawn.
 *
 * Never pixel-equality and never the drawn English strings: the app renders
 * German chrome, so a copy change must not fail a layout suite, and a font
 * substitution must not fail a scale one. The visual assertions are FLOORS —
 * they say "at least this prominent", which is the claim the mockup makes.
 *
 * It runs against a LIVE stack (BASE_URL + a real company id), because the two
 * states that have to look right — a populated account and a freshly imported
 * one — are data states, not mock fixtures. Without BASE_URL the whole suite
 * skips itself loudly rather than passing on an app it never loaded.
 */

const BASE_URL = process.env.BASE_URL;
// The two cases from the plan: an account with deals, activities and a
// dossier, and one that arrived by import with nothing on it yet. Both must
// look right, and they fail differently — the populated one by missing
// regions, the sparse one by showing regions that should be absent.
const POPULATED_COMPANY = process.env.E2E_COMPANY_POPULATED;
const SPARSE_COMPANY = process.env.E2E_COMPANY_SPARSE;

// A live run is opt-in, but a HALF-configured one is a mistake rather than a
// choice: skipping silently there is exactly the failure this suite was built
// to stop, so it fails instead and says which variable is missing.
if (BASE_URL && !(POPULATED_COMPANY && SPARSE_COMPANY)) {
  throw new Error(
    "BASE_URL is set, so this suite runs live — it also needs E2E_COMPANY_POPULATED and E2E_COMPANY_SPARSE (company uuids on that stack).",
  );
}

test.skip(
  !BASE_URL,
  "company-record runs against a live stack: set BASE_URL, E2E_COMPANY_POPULATED and E2E_COMPANY_SPARSE (see make e2e-company).",
);

const SHOTS = process.env.E2E_SHOT_DIR ?? "/tmp/e2e-company";

// The figures plate at the foot of How it stands, by the test id it hands its
// StatStrip. Not by class. The plate is the shared primitive's
// (design-system/statstrip.tsx). A class selector here would name the
// primitive and match the contact record's row too. Or it would name a screen
// class that only exists to be selected. The test id names this row on this page. That is
// what these assertions are about.
const STRIP = '[data-testid="company-figures"]';

/**
 * Open a company and wait for the 360 to settle.
 *
 * The page is a composite of independent reads, so `networkidle` alone can land
 * while cards are still empty. Anchoring on the heading means the assertions
 * below describe a rendered page rather than a racing one.
 */
async function openCompany(page: Page, companyId: string) {
  await page.goto(`/#/companies/${companyId}`, { waitUntil: "networkidle" });
  // The RECORD's own identity block. On a record route the shell's page head
  // shows only the trail back and no heading, and that trail renders from the
  // router before any company read returns — so anchoring there would say the
  // page is settled while every card below it is still empty.
  await expect(page.locator(".record-split-head h1")).toBeVisible();
}

/** The vertical position of a region, for order assertions. */
async function topOf(locator: Locator): Promise<number> {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  if (!box) {
    throw new Error("region is visible but has no box — cannot order it");
  }
  return box.y;
}

test.describe("company record — the glance's page shape", () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page);
  });

  // The readings are the overview's, under the bar that chose it (DESIGN.md
  // §7): a tab without them moves nothing above itself, and a row of account
  // readings over the Contacts roster would be a header for a page it is not
  // describing.
  test("the readings row sits under the tab strip, on the overview alone", async ({
    page,
  }) => {
    await openCompany(page, POPULATED_COMPANY as string);
    expect(await topOf(page.locator("[data-testid='co-tabs']"))).toBeLessThan(
      await topOf(page.locator(STRIP)),
    );
    await page.getByRole("button", { name: "Kontakte" }).click();
    await expect(page.locator(STRIP)).toHaveCount(0);
  });

  // Next meeting, last contact, money and open pipeline on every account. A
  // slot with no reading says so rather than vanishing.
  //
  // Then every slot is checked for a label AND a value: a reading that drew one
  // of the pair and not the other is a door the eye sweeps past.
  test("every reading carries a label and a value", async ({ page }) => {
    await openCompany(page, POPULATED_COMPANY as string);
    const slots = page.locator(`${STRIP} > *`);
    // Waited for, not counted straight away: a bare count() resolves against
    // whatever is mounted at that instant, which on a composite read is an
    // empty strip.
    await expect(slots.nth(1)).toBeVisible();
    const count = await slots.count();
    expect(count).toBe(4);
    for (let index = 0; index < count; index++) {
      const slot = slots.nth(index);
      await expect(slot.locator(".stat-card-label")).not.toBeEmpty();
      await expect(slot.locator(".stat-card-value")).not.toBeEmpty();
    }
  });

  test("the tab strip offers every tab, plus partner only where the account has one", async ({
    page,
  }) => {
    await openCompany(page, POPULATED_COMPANY as string);
    const isPartnerAccount = await page.evaluate(async () => {
      const id = location.hash.split("/").pop();
      const response = await fetch(`/v1/companies/${id}`, {
        headers: { accept: "application/json" },
      });
      const company = await response.json();
      return Boolean(company?.relationship_types?.includes("partner"));
    });
    await expect(
      page
        .locator("[data-testid='co-tabs']")
        .getByRole("button", { name: "Finanzen", exact: true }),
    ).toBeVisible();
    await expect(
      page.locator("[data-testid='co-tabs'] .recordtabs-tab"),
    ).toHaveCount(isPartnerAccount ? 9 : 8);
  });

  test("the overview stacks the account brief, needs and questions, with commercial detail on Deals", async ({
    page,
  }) => {
    await openCompany(page, POPULATED_COMPANY as string);
    const call = page.getByRole("heading", { name: "Unternehmensbericht" });
    const needs = page.getByRole("heading", { name: "Handlungsbedarf" });
    const ask = page.getByRole("heading", {
      name: "Fragen zu diesem Unternehmen",
    });
    await expect(call).toBeVisible();
    await expect(needs).toBeVisible();
    await expect(ask).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Nächste Schritte", exact: true }),
    ).toHaveCount(0);
    expect(await topOf(call)).toBeLessThan(await topOf(needs));
    expect(await topOf(needs)).toBeLessThan(await topOf(ask));
    await expect(
      page.getByRole("heading", { name: "Geschäftliches" }),
    ).toHaveCount(0);
    await page
      .locator("[data-testid='co-tabs']")
      .getByRole("button", { name: /^Deals und Projekte/ })
      .click();
    await expect(
      page.getByRole("heading", { name: "Deals", exact: true }),
    ).toBeVisible();
  });

  // The needs list asks for a move on the account, so it belongs to the tab a
  // reader opens to be told what to do — not to every tab. Someone who has
  // gone to Contacts or Documents has already chosen what to read.
  test("the needs list leads the overview, and is not drawn on the other tabs", async ({
    page,
  }) => {
    await openCompany(page, POPULATED_COMPANY as string);
    const needs = page.getByRole("heading", { name: "Handlungsbedarf" });
    await expect(needs).toBeVisible();

    await page.getByRole("button", { name: "Kontakte" }).click();
    await expect(needs).toHaveCount(0);
  });

  // The details stand in the left card under the account's head, always open:
  // flat groups with a small label each, no titled cards. The work stands to
  // their right. The German copy inside is not pinned.
  test("the details stand beside the work as flat groups, on the left", async ({
    page,
  }) => {
    await openCompany(page, POPULATED_COMPANY as string);
    const details = page.locator(".record-aside .recorddetails");
    await expect(details).toBeVisible();
    await expect(details.locator(".panel")).toHaveCount(0);
    const groups = details.locator(".recorddetails-group");
    await expect(groups.first()).toBeVisible();
    expect(await groups.count()).toBeGreaterThanOrEqual(2);

    const detailsBox = await details.boundingBox();
    const bodyBox = await page.locator(".co-overview-stack").boundingBox();
    if (!detailsBox || !bodyBox) {
      throw new Error("details and stack are visible but one has no box");
    }
    expect(detailsBox.x).toBeLessThan(bodyBox.x);
  });

  // A freshly imported company is lifecycle `unknown` and has nothing on it.
  // It must still render the page's skeleton — the regions carry their own
  // empty states, and a sparse account that drops whole regions is the
  // second way this page stops looking like the design.
  test("an imported company keeps the page's shape", async ({ page }) => {
    await openCompany(page, SPARSE_COMPANY as string);
    await expect(page.locator("[data-testid='co-tabs']")).toBeVisible();
    await expect(page.locator(STRIP)).toBeVisible();
    expect(await topOf(page.locator("[data-testid='co-tabs']"))).toBeLessThan(
      await topOf(page.locator(STRIP)),
    );
    // The readings here too, on an account that can answer almost none of
    // them: each slot says which reading it has none of.
    await expect(page.locator(`${STRIP} > *`).nth(2)).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Unternehmensbericht" }),
    ).toHaveCount(1);
  });

  // Not an assertion — the artifact a human compares against the design. It is
  // written outside the repo (E2E_SHOT_DIR) because a screenshot is session
  // debris, not product.
  test("capture both states for eyeball comparison", async ({ page }) => {
    for (const [name, company] of [
      ["populated", POPULATED_COMPANY],
      ["sparse", SPARSE_COMPANY],
    ] as const) {
      await openCompany(page, company as string);
      // openCompany waits for the h1, which the router paints before the
      // composite read returns. The strip's second slot comes from that read,
      // so its arrival is the proxy for a settled page.
      await expect(page.locator(`${STRIP} > *`).nth(1)).toBeVisible();
      await page.screenshot({
        path: `${SHOTS}/company-${name}.png`,
        fullPage: true,
      });
    }
  });
});

// Live geometry complements the structural and record-header unit tests.
test.describe("company record — visual weight", () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page);
    await openCompany(page, POPULATED_COMPANY as string);
  });

  test("the company's mark is a logo, not a favicon", async ({ page }) => {
    // ~110px square in the mockups, beside a name at ~40px. At 44 it reads as
    // a list-row avatar that wandered onto a record page.
    const box = await page
      .locator(".record-split-head .avatar")
      .first()
      .boundingBox();
    if (!box) {
      throw new Error("the header avatar has no box");
    }
    expect(box.width).toBeGreaterThanOrEqual(72);
  });

  test("the lifecycle control is a control, not a tag", async ({ page }) => {
    // A filled button of ~190x48 in State A, sitting beside the name. The
    // 75x22 pale chip reads as metadata a reader cannot act on.
    // The header draws it as the design system's dropdown worn as a button
    // (Select's `button` appearance), not as InlineChoice: that primitive is a
    // value inside a line of text and is 24px by design, so the header
    // stopped using it as its control (#6033). Reached by the test id the
    // header hands the trigger, for the reason the readings row above is:
    // `.co-standing` was this page's own class and the page stopped drawing
    // it, so the walk found nothing and threw naming a CONTROL — which sends a
    // reader looking for a missing control rather than a missing selector. The
    // primitive's own class would match every other screen's dropdown, and the
    // copy inside it is German chrome this suite deliberately does not pin.
    const box = await page
      .locator('[data-testid="company-lifecycle"]')
      .first()
      .boundingBox();
    if (!box) {
      throw new Error(
        "no [data-testid='company-lifecycle'] on the company header — the id moved, or the control did",
      );
    }
    expect(Math.round(box.height)).toBeGreaterThanOrEqual(32);
  });

  // Website, LinkedIn, location, industry, size: five pills under the
  // description in both mockups. Each is drawn only where the field is
  // recorded — a chip invented for an empty column would state a fact the
  // record does not hold — so this asserts the row renders what the fixture
  // HAS, rather than a fixed count the data may not support.
  test("the header carries the company's attribute chips", async ({ page }) => {
    const recorded = await page.evaluate(async () => {
      const id = location.hash.split("/").pop();
      const response = await fetch(`/v1/companies/${id}`, {
        headers: { accept: "application/json" },
      });
      const company = await response.json();
      return ["industry", "address_city", "linkedin_url"].filter((field) =>
        Boolean(company?.[field]),
      ).length;
    });
    await expect
      .poll(() => page.locator(".record-split-lines .chip").count())
      .toBeGreaterThanOrEqual(recorded);
  });

  test("a card reads as a card against the page behind it", async ({
    page,
  }) => {
    // The mockups set white cards on a light grey page. The border is what
    // makes a card an object; against a near-white background at 1px of very
    // low contrast it stops being visible at all.
    const card = page.locator(".co-overview-stack .panel").first();
    const pageBg = await page.evaluate(
      () => getComputedStyle(document.body).backgroundColor,
    );
    const cardBg = await card.evaluate(
      (element) => getComputedStyle(element).backgroundColor,
    );
    expect(cardBg).not.toBe(pageBg);
  });
});
