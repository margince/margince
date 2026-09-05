import { expect, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import type { MessageKey } from "../src/i18n/en";
import { mockApi } from "./seed";

/**
 * The Analytics screen, section by section.
 *
 * The behavioural tests (vitest) already prove which rows a card maps and which
 * absence draws which plate. What none of them can see is whether a reader
 * opening #/analytics reaches those sections at all, whether a section that has
 * nothing to say SAYS so, and whether the answer survives a reload and a copied
 * link.
 *
 * THE ONE ASSERTION THAT MATTERS MOST is that nothing renders as zero when it
 * means something else. "Too few to say", "no project has gone quiet", "not
 * checked yet" and "0" are four different facts, and an analytics screen that
 * collapses them is worse than one that shows nothing: a manager acts on a
 * number that was never measured. The fixtures are built to put an honest
 * absence on screen BESIDE a real figure, so a screen that drew zeroes would
 * fail here rather than look tidy.
 *
 * German, because the app renders German — asserting the English strings would
 * pass against a build no user sees. Section names come from the translation
 * table itself rather than being retyped, so a copy change moves both.
 */

const t = (key: MessageKey) => de[key];

/**
 * Open Analytics with the API mocked.
 *
 * mockApi alone, and NOT signIn: the mock answers /me with a seated reader, so
 * the app is already authenticated when the first navigation lands. Signing in
 * on top of that walked the app through the login flow and left every
 * subsequent hash navigation on the not-found card — which every assertion here
 * then passed against, because a page that renders nothing overflows nothing
 * and contains none of the strings a negative check looks for.
 *
 * The other specs that reach this screen (history.spec.ts) do the same.
 */
async function openAnalytics(page: Page, section?: string) {
  await mockApi(page);
  await page.goto(`/#/analytics${section ? `/${section}` : ""}`, {
    waitUntil: "networkidle",
  });
}

test.describe("Analytics sections", () => {
  test("every declared section draws its own answer", async ({ page }) => {
    await openAnalytics(page);
    // The strip is the screen's own contract: a section it declares and does
    // not draw is a control that goes nowhere, which is how a reader learns to
    // distrust the whole screen.
    //
    // A GROUP of pressable buttons, not a tablist, and the design system says
    // why where it builds them: a tablist promises arrow-key movement between
    // panels wired by id, and these route to a page. Asserting the role the
    // component actually renders is the difference between testing the screen
    // and testing a guess about it.
    const strip = page.getByRole("group", { name: t("analytics.sections") });
    await expect(strip).toBeVisible();
    for (const key of [
      "analytics.sectionForecast",
      "analytics.sectionPipeline",
      "analytics.sectionPerformance",
      "analytics.sectionDelivery",
    ] as const) {
      await expect(
        strip.getByRole("button", { name: t(key) }),
        `the ${t(key)} section has no control`,
      ).toBeVisible();
    }

    // And each one DRAWS something when pressed. A strip of controls that all
    // route to the same body — or to none — passes a visibility check and fails
    // every reader, so the tabs are pressed rather than counted.
    for (const [key, expected] of [
      ["analytics.sectionPerformance", "analytics.reportStageAge"],
      ["analytics.sectionDelivery", "analytics.reportProjectsByPhase"],
    ] as const) {
      await strip.getByRole("button", { name: t(key) }).click();
      await expect(
        page.getByText(t(expected)).first(),
        `pressing ${t(key)} drew no ${t(expected)} — the control routes nowhere`,
      ).toBeVisible();
    }
    // Two sections are CONDITIONAL and their absence here is the screen being
    // careful rather than incomplete:
    //
    //   - My outcomes appears only for a reader whose own lens is the default,
    //     because it is that reader's own week and nobody else's;
    //   - Data coverage appears only with the ops grant AND after the server
    //     has answered, because a tab that opens onto a refusal is worse than
    //     no tab.
    //
    // This fixture's reader measures the workspace and the mock serves no
    // coverage probe, so neither is drawn. Asserting their ABSENCE keeps the
    // rule visible: a build that showed an ops tab to everyone would fail
    // here rather than look complete.
    //
    // The four assertions above are what makes this safe to assert. A count of
    // zero passes on a page that rendered nothing at all, so an absence is only
    // evidence once something PRESENT has been seen on the same strip — which
    // is why these come after the loop rather than standing alone.
    for (const key of [
      "analytics.sectionOutcomes",
      "analytics.sectionCoverage",
    ] as const) {
      await expect(
        strip.getByRole("button", { name: t(key) }),
        `${t(key)} is drawn for a reader who cannot use it`,
      ).toHaveCount(0);
    }
  });

  test("a section's answer survives a reload and a copied link", async ({
    page,
  }) => {
    await openAnalytics(page, "delivery");
    const delivery = page.getByRole("button", {
      name: t("analytics.sectionDelivery"),
    });
    await expect(delivery).toHaveAttribute("aria-pressed", "true");

    // A reader who reloads, or pastes the link to a colleague, must land on the
    // same answer. A section held only in memory sends them back to the default
    // and quietly answers a different question than the one they shared.
    await page.reload({ waitUntil: "networkidle" });
    await expect(
      page.getByRole("button", { name: t("analytics.sectionDelivery") }),
      "the section was lost on reload — a copied link answers a different question",
    ).toHaveAttribute("aria-pressed", "true");
    // And the BODY came back with it. aria-pressed is computed from the route
    // alone, so it stays true over a section that rendered nothing — which is
    // the state a reader following a shared link would actually be in.
    await expect(
      page.getByText(t("analytics.reportProjectsByPhase")).first(),
      "the pressed section drew no report after reload — the link restores a " +
        "tab and not an answer",
    ).toBeVisible();
  });

  test("a figure too small to report says so rather than showing zero", async ({
    page,
  }) => {
    await openAnalytics(page, "performance");
    // The stage-age fixture holds one stage with a real median (12 days) and one
    // with too few deals to have one. BOTH are asserted: the number proves the
    // card rendered and read its rows, and the words prove it does not fill an
    // absence with a zero.
    //
    // The number first, because without it the words below could come from a
    // card that failed to render anything at all.
    // Scoped to the ROW, not to a container: DataTable's scroll region is
    // labelled only when its content overflows, so there is no stable wrapper to
    // find the card by. The row names its own stage, which is what makes these
    // two assertions about the same table.
    //
    // Both, and in this order. The measured stage's median proves the card read
    // its rows at all; without it the words below could come from a card that
    // rendered nothing, or from win-loss, which draws the same DaysCell.
    const measured = page.getByRole("row", { name: /Qualify/ });
    await expect(
      measured.getByText(t("analytics.days").replace("{days}", "12")),
      "the measured stage drew no median — the card did not read its rows",
    ).toBeVisible();

    const unmeasured = page.getByRole("row", { name: /Proposal/ });
    await expect(
      unmeasured.getByText(t("analytics.tooFewForMedian")).first(),
      "a stage under the sample floor drew no words — a reader cannot tell an " +
        "unmeasured stage from a fast one",
    ).toBeVisible();
  });

  test("a section with nothing to report says so in words", async ({ page }) => {
    await openAnalytics(page, "delivery");
    // projects-gone-quiet answers no rows, which is a real answer — "nothing
    // has gone quiet" — and an empty table would leave a reader wondering
    // whether the check ran.
    await expect(
      page.getByText(t("analytics.nothingQuiet")).first(),
      "an empty result drew an empty table; a reader cannot tell that from a " +
        "check that never ran",
    ).toBeVisible();
  });

  test("the page does not scroll sideways at a phone width", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openAnalytics(page, "performance");
    // The content has to BE there before its width means anything: a blank body
    // overflows nothing and would pass this by rendering less than the screen
    // promises.
    await expect(
      page.getByText(t("analytics.reportStageAge")).first(),
      "the performance section drew nothing, so measuring its width proves nothing",
    ).toBeVisible();

    // BOTH scrollers, not the document alone. The shell pins `.main` to
    // overflow:hidden and scrolls content through `.scroll`, so wide content
    // rides inside the page's own scroller while the document reports zero —
    // ac.spec.ts records measuring 273px of overflow there against a body that
    // never grew a pixel. A document-only check is structurally incapable of
    // failing.
    //
    // A component's OWN scroll region is deliberately not measured: a table too
    // wide for a phone scrolling inside its card is the sanctioned answer to
    // wide content, and the page around it never moves.
    const overflowing = await page.evaluate(() =>
      [
        { name: "the document", element: document.documentElement },
        ...Array.from(document.querySelectorAll(".scroll")).map((element) => ({
          name: "the shell content scroller (.scroll)",
          element,
        })),
      ]
        .map(({ name, element }) => ({
          name,
          overflow: element.scrollWidth - element.clientWidth,
        }))
        .filter(({ overflow }) => overflow > 1)
        .map(({ name, overflow }) => `${name}: ${overflow}px past the viewport`),
    );
    expect(
      overflowing,
      "the page scrolls sideways at 390px — a column is out of reach",
    ).toEqual([]);
  });
});
