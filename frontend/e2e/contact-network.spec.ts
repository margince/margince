import { expect, type Page, test } from "@playwright/test";
import { signIn, textsOf } from "./waits";

/**
 * The contact Network tab against the layout it was rebuilt to.
 *
 * It exists for the reason the company-record suite exists: "it looks like the
 * design" was reported twice from reading the code rather than the page, and
 * both times the page did not look like it. Shape is asserted first — which
 * regions exist and in what order — then the two things a screenshot would
 * catch and a DOM query would not: the lead panel actually reading as the
 * page's one lead, and the two columns actually being two columns.
 *
 * It runs against a LIVE stack (BASE_URL + a real contact id with routes),
 * because a route candidate comes from captured correspondence and a seeded
 * database has none. Without BASE_URL the suite skips itself loudly rather
 * than passing on an app it never loaded.
 */

const BASE_URL = process.env.BASE_URL;
// A contact somebody here actually corresponds with. Without routes the tab is
// its empty state, which is a different page and not the one under test.
const CONTACT_ID = process.env.E2E_CONTACT_ID;

test.skip(
  !BASE_URL || !CONTACT_ID,
  "set BASE_URL and E2E_CONTACT_ID to run the Network tab layout suite",
);

/**
 * Open the tab and wait for the graph read to settle.
 *
 * The tab is a composite of two independent reads, so `networkidle` can land
 * while the chips are still drawing. Anchoring on the jump chips means every
 * assertion below describes a rendered page rather than a racing one.
 */
async function openNetwork(page: Page) {
  await page.goto(`/#/contacts/${CONTACT_ID}/network`, {
    waitUntil: "networkidle",
  });
  await expect(page.locator(".chapter-jump").first()).toBeVisible();
}

test.describe("Network tab — page shape", () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page);
    await openNetwork(page);
  });

  test("leads with a chip per chapter, then the recommended route", async ({
    page,
  }) => {
    // Way in, who knows them, progress. Fewer means a chapter failed to
    // render rather than that this record lacks the fact.
    await expect(
      page.locator(".chapter-jump").first().locator(".chapter-jump-chip"),
    ).toHaveCount(3);

    // The page has one lead. A second would ask the reader which
    // recommendation to believe.
    await expect(page.locator(".panel:has(.pn-verdict)")).toHaveCount(1);
  });

  test("draws the map and no ego ring", async ({ page }) => {
    await expect(page.locator(".rmap").first()).toBeVisible();
    // The retired drawing. Its classes surviving would mean the old component
    // is still mounted somewhere on this page.
    await expect(page.locator(".pn-ring, .pn-node, .pn-edge")).toHaveCount(0);
  });

  test("reads as three chapters, the handoff in the last", async ({ page }) => {
    await expect(page.locator(".pn-stack .chapter")).toHaveCount(3);
    // The four-step relay is an ordered list, so a screen reader hears
    // "step 2 of 4" rather than a row of circles.
    await expect(
      page.locator(".pn-stack .chapter").last().locator("ol.pn-relay li"),
    ).toHaveCount(4);
  });

  /**
   * All three lanes draw.
   *
   * The map's centre column is for ONE hub node and pushes no lane head, so
   * putting the target's twelve colleagues there drew a 1000px column with two
   * of the three lanes missing and eleven nodes painted off the canvas. The
   * DOM said eleven nodes either way — only the lane heads tell the two apart.
   */
  test("draws our team, their company and the target", async ({ page }) => {
    // The centre column takes ONE hub node and pushes no lane head, so the
    // target belongs there and the company's contacts belong in a right lane
    // that DOES get a head. Putting the twelve contacts in the centre drew a
    // 1000px column, no head for them, and their nodes off the canvas.
    //
    // The lane heads are what tell the two arrangements apart: the node count
    // is identical either way, which is how the first version of this test
    // passed against the broken layout.
    const lanes = await textsOf(page.locator(".rmap text.rmap-lane"));
    expect(lanes).toContain("Dein Team");
    expect(lanes).toContain("Unternehmen des Kontakts");

    // Every node the model carries is inside the drawing's own viewBox. A node
    // painted past it is in the DOM, passes every count, and is invisible.
    const outside = await page.evaluate(() => {
      const svg = document.querySelector(".rmap-svg");
      if (!svg) {
        return -1;
      }
      const vb = svg.getAttribute("viewBox")?.split(" ").map(Number) ?? [];
      const w = vb[2] ?? 0;
      return [...svg.querySelectorAll("[data-node-id] rect")].filter((r) => {
        const x = Number(r.getAttribute("x") ?? 0);
        const rw = Number(r.getAttribute("width") ?? 0);
        return x + rw > w + 1;
      }).length;
    });
    expect(outside).toBe(0);
  });

  test("offers the ask on the lead route", async ({ page }) => {
    const lead = page.locator(".panel:has(.pn-verdict)").first();
    const ask = lead.locator(".pn-verdict-actions button");
    const ownRoute = lead.getByText(
      "Es gibt niemanden, den du fragen kannst. Schreibe diesem Kontakt direkt.",
      { exact: true },
    );
    const ownsRecommendedRoute = await page.evaluate(async (contactId) => {
      const [meResponse, graphResponse] = await Promise.all([
        fetch("/v1/me"),
        fetch(`/v1/contacts/${contactId}/graph`),
      ]);
      if (!meResponse.ok || !graphResponse.ok)
        throw new Error("the route and signed-in user must be readable");
      const me = await meResponse.json();
      const graph = await graphResponse.json();
      const recipient = graph.routes?.[0]?.via_user_id;
      if (!recipient || !me.user?.id)
        throw new Error("the live fixture needs a named recommended route");
      return recipient === me.user.id;
    }, CONTACT_ID);
    if (ownsRecommendedRoute) {
      await expect(ownRoute).toBeVisible();
      await expect(ask).toHaveCount(0);
      await expect(lead.locator(".pn-hero-who strong").first()).toHaveText(
        "Du",
      );
    } else {
      await expect(ownRoute).toHaveCount(0);
      await expect(ask).toHaveCount(1);
      await expect(ask).toBeEnabled();
    }
  });
});

test.describe("Network tab — visual weight", () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page);
    await openNetwork(page);
  });

  /**
   * The strip and the lead panel sit ABOVE the map.
   *
   * This is the whole complaint the rebuild answered: the answer a rep opens
   * the tab for used to be three cards down. Order in the DOM is not enough —
   * a floated or absolutely positioned block can read in source order and
   * paint anywhere — so this reads the painted geometry.
   */
  test("the answer is above the picture", async ({ page }) => {
    const answer = await page.locator(".chapter-answer").first().boundingBox();
    const lead = await page
      .locator(".panel:has(.pn-verdict)")
      .first()
      .boundingBox();
    const map = await page.locator(".rmap").first().boundingBox();
    if (!answer || !lead || !map) {
      throw new Error("the answer, lead and map need visible boxes");
    }
    expect(answer.y).toBeLessThan(lead.y);
    expect(lead.y).toBeLessThan(map.y);
  });

  // One column at every width: the chapters keep their order rather than
  // folding into a grid that pushes the handoff beside the map.
  for (const width of [900, 1920]) {
    test(`the chapters stack in order at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 });
      const tops = await page
        .locator(".pn-stack .chapter")
        .evaluateAll((chapters) =>
          chapters.map((c) => c.getBoundingClientRect().top),
        );
      expect(tops).toHaveLength(3);
      expect([...tops].sort((x, y) => x - y)).toEqual(tops);
    });
  }

  /** Nothing may push the page sideways. */
  test("the page does not scroll horizontally", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    const overflow = await page.evaluate(
      () =>
        document.documentElement.scrollWidth -
        document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(1);
  });
});
