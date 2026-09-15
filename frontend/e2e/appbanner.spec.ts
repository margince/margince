import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * The shell's standing advisory, rendered — which nothing had ever done.
 *
 * `EmbedReindexBanner` keys on `/embeddings/reindex/status`, and no mock
 * answered that read: the query errored, the component returned null, and every
 * sweep in this suite — the AC screens, the axe pass, the overflow pass — ran
 * against a shell with no banner in it. A whole mounted surface, on every route
 * at once, that the harness could not see.
 *
 * So `mockApi` grew `embedReindex`, and this is the first thing to use it. What
 * it holds is what a sweep holds: the advisory is DRAWN when the identities
 * disagree and absent when they agree, it names itself and its way onward to a
 * screen reader, and it survives the same accessibility bar as the page it
 * stands over — at both widths, because the chrome above the content column is
 * not the same chrome at 390px.
 */

const BANNER = ".appbanner";

const WIDTHS = [
  { name: "desktop", size: { width: 1440, height: 900 } },
  { name: "390px", size: { width: 390, height: 844 } },
];

async function open(page: Page, route: string, reindex: "current" | "needed") {
  await mockApi(page, { embedReindex: reindex });
  await page.goto(route);
  // The page itself, before the banner: a shell that failed to render would
  // otherwise report an absent banner as the state under test.
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
}

test.describe("the reindex advisory", () => {
  for (const width of WIDTHS) {
    test(`is drawn once, above the page, at ${width.name}`, async ({
      page,
    }) => {
      await page.setViewportSize(width.size);
      await open(page, "#/settings/knowledge", "needed");

      // Once. It is mounted in the shell's content column rather than by a
      // screen, so a second copy would mean a screen had grown its own.
      await expect(page.locator(BANNER)).toHaveCount(1);
      // Above the page's own heading, in flow. The column reaches UP behind the
      // top bar (`.main:not(:has(> .appbanner)) > .scroll`, app/shell.css) and a
      // banner has to turn that off; if it did not, the heading would start
      // inside the banner standing over it.
      const banner = await page.locator(BANNER).boundingBox();
      const heading = await page
        .getByRole("heading", { level: 1 })
        .first()
        .boundingBox();
      expect(banner).not.toBeNull();
      expect(heading).not.toBeNull();
      if (!banner || !heading) {
        return;
      }
      expect(
        heading.y,
        "the page's own heading starts inside the advisory standing over it",
      ).toBeGreaterThanOrEqual(banner.y + banner.height);
    });
  }

  // The absence is the other half, and it is the half a mock gets wrong: a
  // fixture that showed the banner unconditionally would leave every other
  // spec measuring a shell no installation in this state has.
  test("is absent while the index matches the model", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await open(page, "#/settings/knowledge", "current");
    await expect(page.locator(BANNER)).toHaveCount(0);
  });

  // A standing notice a reader cannot act on is worse than none: the link is
  // the whole of what it offers.
  test("names itself and its way onward", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await open(page, "#/settings/knowledge", "needed");

    const banner = page.locator(BANNER);
    await expect(banner).toContainText(/\S/);
    await expect(
      banner.getByRole("link"),
      "the advisory offers no way to the screen that resolves it",
    ).toHaveAttribute("href", /settings/);
  });

  test("meets the same accessibility bar as the page it stands over", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await open(page, "#/settings/knowledge", "needed");

    const results = await new AxeBuilder({ page })
      .include(BANNER)
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
      .analyze();
    expect(
      results.violations.map((violation) => violation.id),
      "the advisory violates the bar every screen under it passes",
    ).toEqual([]);
  });
});
