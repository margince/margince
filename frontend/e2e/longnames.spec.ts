import { expect, type Page, test } from "@playwright/test";
import { brandt } from "./mockapi/fixtures";
import { page as listPage } from "./mockapi/server";
import { mockApi } from "./seed";
import { pageOverflow } from "./waits";

/**
 * A name nobody broke into words stays inside the page that shows it.
 *
 * A company name copied from a URL is one unbroken run. A flex or grid cell
 * sizes to its content unless told otherwise, so the run pushed its neighbours
 * off the window. Only a browser lays out the box; jsdom does not.
 */

const LONG = `https://example.test/${"a".repeat(200)}`;

async function openDealWithLongCompany(page: Page, width: number) {
  await page.setViewportSize({ width, height: 800 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mockApi(page);
  await page.route(/\/v1\/companies\/o-brandt$/, async (route) => {
    await route.fulfill({ json: { ...brandt, display_name: LONG } });
  });
  await page.goto("/#/deals/d-fleet");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByText(LONG).first()).toBeVisible();
}

for (const width of [1280, 390]) {
  test.describe(`a deal page at ${width}px`, () => {
    test("does not pan for a company name with no break in it", async ({
      page,
    }) => {
      await openDealWithLongCompany(page, width);
      expect(await pageOverflow(page)).toEqual([]);
    });

    test("keeps the company name inside its card", async ({ page }) => {
      await openDealWithLongCompany(page, width);
      // The record's own nodes: the agent rail beside it is app chrome that
      // clamps its own lines and echoes the page it is on.
      const verdicts = await page
        .locator(".record-split")
        .getByText(LONG)
        .evaluateAll((nodes) =>
          nodes.map((node) => {
            const range = document.createRange();
            range.selectNodeContents(node);
            // A name that ellipsises clips its own overflow, and a range over
            // its text still measures the whole unclipped run. What the reader
            // sees ends at the node's own edge.
            const clips = getComputedStyle(node).overflowX !== "visible";
            const shown = clips ? node : range;
            const card = node.closest(
              ".panel, .record-split-details, .record-split-sheet",
            );
            return {
              node: `${node.tagName}.${node.className}`,
              card: card?.className ?? null,
              past: card
                ? shown.getBoundingClientRect().right -
                  card.getBoundingClientRect().right
                : null,
            };
          }),
        );
      expect(verdicts.length).toBeGreaterThan(0);
      for (const verdict of verdicts) {
        expect(verdict.card, `${verdict.node} has a card`).not.toBeNull();
        expect(
          verdict.past,
          `${verdict.node} past ${verdict.card}`,
        ).toBeLessThanOrEqual(0.5);
      }
    });
  });
}

const NAMES = [
  "Brandt Automotive Fleet Services International Holdings and Logistics Alpha",
  "Brandt Automotive Fleet Services International Holdings and Logistics Bravo",
];
const ARCHIVED_NAME =
  "Brandt Automotive Fleet Services International Holdings and Logistics Charlie";

async function openCompaniesWithLongNames(page: Page, width: number) {
  await page.setViewportSize({ width, height: 800 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mockApi(page);
  await page.route(/\/v1\/companies(\?.*)?$/, async (route) => {
    await route.fulfill({
      json: listPage(
        [...NAMES, ARCHIVED_NAME].map((display_name, i) => ({
          ...brandt,
          id: `o-long-${i}`,
          display_name,
          archived_at:
            display_name === ARCHIVED_NAME ? "2026-06-20T09:00:00Z" : null,
        })),
      ),
    });
  });
  await page.goto("/#/companies");
}

async function nameCell(page: Page, name: string) {
  const cell = page.locator("td strong", { hasText: name });
  await expect(cell).toBeVisible();
  await expect(cell).toHaveAttribute("title", name);
  return cell.evaluate((node) => {
    const td = node.closest("td");
    return {
      textOverflow: getComputedStyle(node).textOverflow,
      truncated: node.scrollWidth > node.clientWidth,
      past: td
        ? node.getBoundingClientRect().right - td.getBoundingClientRect().right
        : null,
    };
  });
}

/** How far the archived badge's words run past the box that shows them. */
async function archivedBadgeCut(page: Page) {
  const label = page
    .locator("tr", { hasText: ARCHIVED_NAME })
    .locator(".avatar-row .badge .badge-label");
  await expect(label).toBeVisible();
  // Not scrollWidth: it rounds to whole pixels, and a label short by a
  // fraction of one already ends in "…".
  return label.evaluate((node) => {
    const words = document.createRange();
    words.selectNodeContents(node);
    return (
      words.getBoundingClientRect().width - node.getBoundingClientRect().width
    );
  });
}

for (const width of [768, 1024, 1440]) {
  test(`the companies list ends a long name in an ellipsis at ${width}px`, async ({
    page,
  }) => {
    await openCompaniesWithLongNames(page, width);
    for (const name of [...NAMES, ARCHIVED_NAME]) {
      const verdict = await nameCell(page, name);
      expect(verdict.textOverflow, name).toBe("ellipsis");
      expect(verdict.truncated, `${name} is truncated`).toBe(true);
      expect(verdict.past, `${name} inside its cell`).toBeLessThanOrEqual(0.5);
    }
    expect(
      await archivedBadgeCut(page),
      "the archived badge keeps its whole label",
    ).toBeLessThanOrEqual(0.01);
    expect(await pageOverflow(page)).toEqual([]);
  });
}

test("the companies list wraps a long name in its card at 390px", async ({
  page,
}) => {
  await openCompaniesWithLongNames(page, 390);
  for (const name of [...NAMES, ARCHIVED_NAME]) {
    const verdict = await nameCell(page, name);
    expect(verdict.truncated, `${name} shows whole`).toBe(false);
    expect(verdict.past, `${name} inside its cell`).toBeLessThanOrEqual(0.5);
    const overlap = await page
      .locator("tr", { hasText: name })
      .evaluate((row) => {
        const heading = row.querySelector("td.lt-identity .lt-identity-row");
        const field = [
          ...row.querySelectorAll("td.lt-identity ~ td:not(.lt-rowmenu)"),
        ].find((td) => td.getClientRects().length > 0);
        return heading && field
          ? heading.getBoundingClientRect().bottom -
              field.getBoundingClientRect().top
          : null;
      });
    expect(overlap, `${name} clear of the field below`).toBeLessThanOrEqual(
      0.5,
    );
  }
  expect(
    await archivedBadgeCut(page),
    "the archived badge keeps its whole label",
  ).toBeLessThanOrEqual(0.01);
  expect(await pageOverflow(page)).toEqual([]);
});

// At the 72px floor the chip, two gaps and the pill's own inset already fill the cell.
const NARROW_COLUMN = 100;

test(`the companies list ends the archived badge in an ellipsis in a ${NARROW_COLUMN}px column`, async ({
  page,
}) => {
  await page.addInitScript((width) => {
    localStorage.setItem(
      "margince.table.widths.v2.unit.companies",
      JSON.stringify({ name: width }),
    );
  }, NARROW_COLUMN);
  await openCompaniesWithLongNames(page, 1440);
  const badge = page
    .locator("tr", { hasText: ARCHIVED_NAME })
    .locator(".avatar-row .badge");
  await expect(badge).toBeVisible();
  const verdict = await badge.evaluate((node) => {
    const td = node.closest("td");
    const label = node.querySelector(".badge-label");
    if (!td || !label) {
      return null;
    }
    const box = td.getBoundingClientRect();
    const inset = Number.parseFloat(getComputedStyle(td).paddingRight);
    return {
      column: box.width,
      past: node.getBoundingClientRect().right - (box.right - inset),
      textOverflow: getComputedStyle(label).textOverflow,
      cut: label.scrollWidth > label.clientWidth,
    };
  });
  expect(verdict?.column, "the stored width holds").toBeCloseTo(
    NARROW_COLUMN,
    0,
  );
  expect(verdict?.past, "the badge inside its cell").toBeLessThanOrEqual(0.5);
  expect(verdict?.textOverflow).toBe("ellipsis");
  expect(verdict?.cut, "the badge label ends in an ellipsis").toBe(true);
});
