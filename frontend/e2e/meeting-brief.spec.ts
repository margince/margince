import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import type { MessageKey } from "../src/i18n/en";
import { type MockApiOptions, mockApi } from "./seed";

// The meeting brief a rep opens two minutes before a room.
//
// What is asserted here is the part no unit test can see: that the drawer's
// bands stay where a reader can reach them, that nothing runs off the side of a
// phone, and that the preparation a reader must not miss is above the parts
// they can scroll to. jsdom resolves no custom properties and measures no
// boxes, so geometry is only ever provable here.

const DRAWER = ".modal-drawer-wide";

function copy(key: MessageKey): string {
  return de[key];
}

// Reduced motion is emulated BEFORE the page loads, not after: an arrival that
// has already started keeps running, and a box measured mid-fade is a box in a
// place the reader never sees it.
//
// The shell's core mark (.core-rim, .core-glass) is excluded by name. Its only
// motion is a 0.3s opacity transition (margince-core.css) — it cannot move a
// box, so it cannot put a measurement anywhere the reader does not see it, and
// it is still in flight whenever the drawer opens over it. Excluded by NAME
// rather than by widening the check to "some animations are fine", which would
// also excuse a drawer still sliding in.
const AMBIENT = ["core-rim", "core-glass"];

async function settleAnimations(page: Page) {
  const running = await page.evaluate((ambient) => {
    const named = (element: Element) =>
      `${element.tagName.toLowerCase()}.${element.className}`;
    return document
      .getAnimations()
      .filter((animation) => animation.playState === "running")
      .map((animation) => {
        const effect = animation.effect;
        return effect instanceof KeyframeEffect &&
          effect.target instanceof Element
          ? named(effect.target)
          : "an element the effect does not name";
      })
      .filter((name) => !ambient.some((mark) => name.includes(mark)));
  }, AMBIENT);
  expect(
    running,
    "an animation was still running when a box was measured, so the box is where the element passed through rather than where it rests",
  ).toEqual([]);
}

// The no-sideways-scroll check, over what THIS surface owns: the document and
// the drawer's own scroll lane.
//
// The shell's content scroller is deliberately not measured here. It carries a
// 16px overflow on the person record's meetings tab with no drawer open at all
// — a pre-existing fault of that page, filed rather than fixed in this change,
// and one that would make this spec fail for a reason the meeting brief cannot
// cause and cannot fix.
async function pageOverflow(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const scrollers: { name: string; element: Element }[] = [
      { name: "the document", element: document.documentElement },
      ...Array.from(document.querySelectorAll(".drawer-body")).map(
        (element) => ({ name: "the drawer's scroll lane", element }),
      ),
    ];
    return scrollers
      .map(({ name, element }) => ({
        name,
        overflow: element.scrollWidth - element.clientWidth,
      }))
      .filter(({ overflow }) => overflow > 0)
      .map(({ name, overflow }) => `${name}: ${overflow}px past the viewport`);
  });
}

async function openBrief(page: Page, options?: MockApiOptions) {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await mockApi(page, options);
  await page.goto("/#/contacts/p-anna/meetings");
  await page
    .getByRole("button", { name: copy("person.meeting.brief") })
    .click();
  await expect(page.locator(DRAWER)).toBeVisible();
  await settleAnimations(page);
}

// A box, or a failure that names what was missing rather than "null".
async function boxOf(page: Page, selector: string) {
  const box = await page.locator(selector).first().boundingBox();
  expect(box, `${selector} has no box on the page`).not.toBeNull();
  return box as NonNullable<typeof box>;
}

test("AC-meeting-brief-1: a rep opens the brief and meets the ask before the detail", async ({
  page,
}) => {
  await openBrief(page);
  await expect(
    page.getByRole("heading", { name: copy("person.meeting.title") }),
  ).toBeVisible();

  // The ask, then the watch-out, then everything else. A reader who stops
  // after the first screen has still met the two that change what they say.
  const goal = await boxOf(page, ".panel-accent");
  const risk = await boxOf(page, ".callout-warn");
  expect(goal.y).toBeLessThan(risk.y);
  await expect(
    page.getByRole("heading", { name: copy("person.meeting.goal") }),
  ).toBeVisible();
});

test("AC-meeting-brief-2: nothing runs off the side of the page", async ({
  page,
}) => {
  await openBrief(page);
  expect(await pageOverflow(page)).toEqual([]);
});

test("AC-meeting-brief-3: the close control stays reachable through a long brief", async ({
  page,
}) => {
  await openBrief(page);
  const drawer = await boxOf(page, DRAWER);
  await page.locator(".drawer-body").evaluate((el) => {
    el.scrollTop = 2000;
  });
  const head = await boxOf(page, ".drawer-head");
  const foot = await boxOf(page, ".drawer-foot");
  // Both bands are sticky: a reader deep in the brief can still close it and
  // still see which meeting they are reading about.
  expect(head.y).toBeGreaterThanOrEqual(drawer.y - 1);
  expect(foot.y + foot.height).toBeLessThanOrEqual(
    drawer.y + drawer.height + 1,
  );
});

test("AC-meeting-brief-4: the brief is wide enough to prepare in", async ({
  page,
}) => {
  await openBrief(page);
  const drawer = await boxOf(page, DRAWER);
  // 52vw of the 1280 the suite pins. A narrower drawer put the close plan's
  // three columns at 100px each, which is not a plan anybody can read.
  expect(drawer.width).toBeGreaterThan(600);
});

test("AC-meeting-brief-5: Escape closes the brief and returns the reader", async ({
  page,
}) => {
  await openBrief(page);
  await page.keyboard.press("Escape");
  await expect(page.locator(DRAWER)).toBeHidden();
  await expect(
    page.getByRole("button", { name: copy("person.meeting.brief") }),
  ).toBeFocused();
});

test("AC-meeting-brief-6: a withheld source says so rather than staying silent", async ({
  page,
}) => {
  await openBrief(page);
  // Background is collapsed, so the reader opens it to find what they are not
  // being shown — but it is THERE, which a silent omission would not be.
  await page.getByText(copy("person.meeting.background")).click();
  await expect(page.getByText(/Deal Room/i).first()).toBeVisible();
});

test("AC-meeting-brief-7: a cold record says nothing is recorded yet", async ({
  page,
}) => {
  await openBrief(page, { meetingBrief: "empty" });
  await expect(page.getByText(copy("person.meeting.empty"))).toBeVisible();
});

test("AC-meeting-brief-8: a failed read offers the reason and a retry", async ({
  page,
}) => {
  await openBrief(page, { meetingBrief: "failed" });
  await expect(
    page.getByText("That meeting is filed under a different engagement."),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: copy("state.retry") }),
  ).toBeVisible();
});

test.describe("with a preparation plan", () => {
  test("AC-meeting-brief-12: the outcome to earn leads, and the sections stay", async ({
    page,
  }) => {
    await openBrief(page, { meetingBrief: "plan" });
    const objective = await boxOf(page, ".panel-accent");
    const close = await boxOf(page, ".mb-advance");
    expect(objective.y).toBeLessThan(close.y);
    // An outline plan ADDS to the brief. The watch-out a reader already had
    // must still be on the page, not buried behind it.
    await expect(page.locator(".callout-warn")).toBeVisible();
  });

  test("AC-meeting-brief-13: the three ways to close sit side by side", async ({
    page,
  }) => {
    await openBrief(page, { meetingBrief: "plan" });
    const legs = await page.locator(".mb-advance > *").all();
    expect(legs).toHaveLength(3);
    const boxes = await Promise.all(legs.map((leg) => leg.boundingBox()));
    const [first, second, third] = boxes.map(
      (box) => box as NonNullable<typeof box>,
    );
    // Same row: a close plan read as three stacked paragraphs is a list, and
    // the point of the three is that a reader compares them at a glance.
    expect(Math.abs(first.y - second.y)).toBeLessThanOrEqual(1);
    expect(Math.abs(second.y - third.y)).toBeLessThanOrEqual(1);
  });
});

test.describe("on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test("AC-meeting-brief-9: the brief is a full-width sheet with nothing cut off", async ({
    page,
  }) => {
    await openBrief(page);
    const drawer = await boxOf(page, DRAWER);
    expect(drawer.width).toBe(390);
    // The three bands pay their own padding on a phone too: a header inset
    // from the sheet's edge reads as a rendering fault.
    const head = await boxOf(page, ".drawer-head");
    expect(head.x).toBe(drawer.x);
    expect(await pageOverflow(page)).toEqual([]);
  });

  test("AC-meeting-brief-14: the close plan stacks on a phone", async ({
    page,
  }) => {
    await openBrief(page, { meetingBrief: "plan" });
    const legs = await page.locator(".mb-advance > *").all();
    const boxes = await Promise.all(legs.map((leg) => leg.boundingBox()));
    const ys = boxes.map((box) => (box as NonNullable<typeof box>).y);
    // Strictly increasing: three columns in 390px would be 110px each, which is
    // a column of broken words rather than a plan.
    expect(ys[1]).toBeGreaterThan(ys[0]);
    expect(ys[2]).toBeGreaterThan(ys[1]);
    expect(await pageOverflow(page)).toEqual([]);
  });

  test("AC-meeting-brief-10: the phone brief has no accessibility violations", async ({
    page,
  }) => {
    await openBrief(page);
    const results = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
      .analyze();
    expect(results.violations).toEqual([]);
  });
});

test.describe("in the dark theme", () => {
  test.use({ colorScheme: "dark" });

  test("AC-meeting-brief-11: the brief reads in the dark theme", async ({
    page,
  }) => {
    await openBrief(page);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    const results = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
      .analyze();
    expect(results.violations).toEqual([]);
  });
});
