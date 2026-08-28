import { expect, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * A record's tab strip stays inside the work column, whatever stands beside it.
 *
 * The strip's rules span the WORK column, so they read as the seam between a
 * record's identity and its content rather than as a box drawn around the
 * tabs. Anything that makes the strip measure itself against a wider box than
 * the column it sits in draws it under whatever occupies the difference.
 *
 * That used to be a rail INSIDE the container, to the left, and the symptom was
 * 138px of strip under it — on the contact page, the first three tabs. Every
 * record now files its context in the shell's own column instead, which is a
 * sibling of the work column rather than a track inside it, so the neighbour
 * moved to the right and the question is the same one: does the strip end where
 * its column ends.
 *
 * A geometry test rather than a unit one, because the defect is layout: the
 * markup, the labels and the order were all correct while the page was wrong.
 * jsdom has no box model and would have passed throughout.
 *
 * EVERY record that carries a context column, not just the one a defect was
 * reported on: the invariant belongs to the strip, and five pages draw it.
 */

// Above the fold the context column is BESIDE the work column and can be run
// under; below 1100px it stacks underneath and the question does not arise.
const RAILED_WIDTHS = [1280, 1440, 1600];

const RECORDS = [
  { name: "contact", route: "/#/contacts/p-anna" },
  { name: "lead", route: "/#/leads/l-1" },
  { name: "company", route: "/#/companies/o-brandt" },
];

async function openRecord(page: Page, route: string) {
  await mockApi(page);
  await page.goto(route);
  await expect(page.locator(".recordtabs-tab").first()).toBeVisible();
  // The column is the shell's, and it is what the strip must not reach under.
  // A test that ran before it filled would measure a page with no neighbour and
  // pass on every record.
  await expect(page.locator(".pageaside")).toBeVisible();
}

test.describe("the record tab strip", () => {
  for (const record of RECORDS) {
    for (const width of RAILED_WIDTHS) {
      test(`is clear of the context column on a ${record.name} at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 900 });
        await openRecord(page, record.route);

        const aside = await page.locator(".pageaside").boundingBox();
        const tabs = await page.locator(".recordtabs-tab").all();
        expect(tabs.length).toBeGreaterThan(0);
        if (!aside) {
          throw new Error("the context column is visible but has no box");
        }

        for (const tab of tabs) {
          const box = await tab.boundingBox();
          const label = (await tab.textContent())?.trim();
          if (!box) {
            throw new Error(`tab "${label}" has no box`);
          }
          // Only a column on the same row can cover a tab. Under the fold the
          // context stacks below, and a right-edge comparison alone would read
          // that stacking as an overlap.
          const sameRow =
            box.y < aside.y + aside.height && box.y + box.height > aside.y;
          if (sameRow) {
            expect(
              box.x + box.width,
              `tab "${label}" ends right of the context column's left edge, so the column covers it`,
            ).toBeLessThanOrEqual(aside.x);
          }
        }
      });
    }
  }

  // The strip belongs to its column, on EVERY record that carries one.
  //
  // Measured against the column rather than against its own children, which is
  // what makes it fail when a breakout returns: a strip and its tabs move
  // TOGETHER under the broken rule, so any assertion comparing the two passes
  // in both states.
  //
  // Only the company and the contact can fail it, and that is a fact about the
  // product rather than a gap here: the breakout lives in `@container work`,
  // which `mainClasses` opens for GRIDDED_RECORD_SCREENS alone, so the lead's
  // strip takes the narrower fallback and has nothing to overhang with. It is
  // swept anyway — if the lead ever joins that set, this is the assertion that
  // has to be true of it on the day it does, and a record left out of the sweep
  // is a record nobody notices is not being measured.
  //
  // 2200px is in the list because the reading column is CAPPED there
  // (--recordColumn) while the work column keeps growing, and a record's column
  // takes no auto margins — so above the cap the column sits at the start of the
  // container rather than in the middle of it. A breakout that centres itself
  // then misses by half the slack, which is tens of pixels and depends on
  // nothing but the two widths. Below the cap the same error is only half a
  // scrollbar wide and a runner with overlay scrollbars cannot see it at all.
  for (const record of RECORDS) {
    for (const width of [1440, 2200]) {
      test(`keeps the width of the column it sits in on a ${record.name} at ${width}px`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 900 });
        await openRecord(page, record.route);

        const strip = await page.locator(".recordtabs").boundingBox();
        const column = await page.locator("main.main").boundingBox();
        if (!strip || !column) {
          throw new Error(
            "the strip and its column are visible but one has no box",
          );
        }
        // One pixel of tolerance: the boxes are laid out in fractional CSS pixels.
        expect(
          strip.x,
          "the strip starts left of its column",
        ).toBeGreaterThanOrEqual(column.x - 1);
        expect(
          strip.x + strip.width,
          "the strip runs past its column's right edge, which is where it reached under the context",
        ).toBeLessThanOrEqual(column.x + column.width + 1);
      });
    }
  }

  // And the first tab opens where the record's own column does.
  //
  // The rules cross the whole work column; the TABS belong to the document under
  // them, so the row insets itself by one gutter and the first tab lands under
  // the record's name. Both halves ride on the same breakout, which is why this
  // is measured against the reading column's content edge rather than against
  // the strip that carries it: when the breakout is misplaced the strip and the
  // row move together, and the first tab opens outside the column — clipped by
  // whatever draws the column's edge, which is how the defect was seen.
  for (const record of RECORDS) {
    test(`opens its first tab at the record column's own edge on a ${record.name}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 2200, height: 900 });
      await openRecord(page, record.route);

      const firstTab = await page
        .locator(".recordtabs-tab")
        .first()
        .boundingBox();
      const columnStart = await page
        .locator(".wrap")
        .first()
        .evaluate((wrap) => {
          const box = wrap.getBoundingClientRect();
          return box.x + Number.parseFloat(getComputedStyle(wrap).paddingLeft);
        });
      if (!firstTab) {
        throw new Error("the first tab is visible but has no box");
      }
      // One pixel, the same tolerance the width assertions take: the boxes are
      // laid out in fractional CSS pixels.
      expect(
        Math.abs(firstTab.x - columnStart),
        "the first tab does not start where the record's column starts",
      ).toBeLessThanOrEqual(1);
    });
  }
});
