import { expect, type Locator, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";
import type { JourneyMark } from "../src/lib/journeymarks";
import {
  type Answer,
  budgetSamples,
  degradedReasons,
  expectationFrom,
  framesLostAfterReturn,
  HOME_READS,
  type Journey,
  longestLoss,
  outcomeOf,
  readerFrom,
  readOf,
  tally,
  withheldSources,
} from "../src/testing/homejourney.judge";
import {
  AnswerLog,
  GLANCE,
  holdWorklist,
  journeyMarks,
  probeClick,
  startJourney,
  watchScreen,
  worklistFetches,
} from "./homejourney";
import { nearestRank } from "./quantile";

/**
 * R01 and R07 on a production build against a live api (`make bench-home`).
 *
 * Three journeys, each repeated SAMPLES times:
 *  - load: a full page load of Home in a signed-in browser. The app starts
 *    cold, the browser cache stays warm. Budgets: useful p95 2 s, complete
 *    p95 2.5 s, and the day and the reader's greeting on screen.
 *  - return: from Home to Contacts and back through the rail, in one
 *    document. Budget: useful again p95 200 ms, and the day never leaves the
 *    screen once it is back.
 *  - refresh: a synthetic `visibilitychange` (a headless browser never hides
 *    a page, so a real tab switch fires none) makes the live reads refetch.
 *    The day's answer is held while the queue button is clicked, so every
 *    click lands during the refetch. The day stays on screen for the same
 *    reader throughout, and the button answers within 200 ms at p95.
 *
 * The times are the app's own journey marks (src/lib/journeymarks.ts). Every
 * journey is also judged by the answers the page was sent
 * (src/testing/homejourney.judge.ts). A journey whose sections answered with
 * a gap counts as degraded, never as complete. A failed or timed-out
 * journey is counted as such, never as a latency.
 */

const SAMPLES = Number(process.env.BENCH_HOME_SAMPLES ?? 20);
const EXPECTED = expectationFrom(process.env.BENCH_HOME_EXPECT ?? "work");
const LOAD_BUDGET_MS = { "home:useful": 2_000, "home:complete": 2_500 };
const RETURN_BUDGET_MS = 200;
const INTERACTION_BUDGET_MS = 200;
const JOURNEY_TIMEOUT_MS = 15_000;
const HOME_MARKS: JourneyMark[] = [
  "home:useful",
  "home:complete",
  "home:degraded",
  "home:failed",
];

// CSS for what runs inside the page (the frame sampler, the click probe),
// where a role query does not exist. "The day is on screen" is the lead card
// of a day with work, or the clear-day plate of a reader who has none.
const DAY_ON_SCREEN = EXPECTED.work
  ? "#brief-today .brief-triage-lead"
  : "#brief-today .surfacestate-empty";
const QUEUE_BUTTON = "#brief-today .record-details-toggle";
const QUEUE_OPEN = `${QUEUE_BUTTON}[aria-pressed="true"]`;
const HOME_LINK = 'nav a[href="#/home"]';

// Roles and labels for what the bench presses, in the German the browser runs
// in. A rail entry is found by its aria-label, because the agent panel inside
// the same landmark can hold a link with the same text.
const railLink = (page: Page, name: string) =>
  page
    .getByRole("navigation", { name: de["shell.railAria"] })
    .getByLabel(name, { exact: true });
const awayLink = (page: Page) => railLink(page, de["nav.contacts"]);
const homeLink = (page: Page) => railLink(page, de["nav.brief"]);
const queueButton = (page: Page, open: boolean): Locator =>
  page.getByRole("button", {
    name: de[open ? "brief.queue.hide" : "brief.queue.show"],
    pressed: open,
  });

type Reader = { id: string; greeting: string | null };

// One worker, in order, and a failed journey does not skip the next: each
// journey's numbers are worth reading whatever the one before it did.
test.describe.configure({ mode: "default" });

async function signIn(page: Page): Promise<Reader> {
  const response = await page.request.post("/v1/auth/login", {
    data: {
      email: process.env.BENCH_HOME_EMAIL ?? "admin@demo.test",
      password: process.env.BENCH_HOME_PASSWORD ?? "demo-password-123",
    },
  });
  expect(response.ok(), `sign-in answered ${response.status()}`).toBe(true);
  await finishOnboarding(page);
  const me = await page.request.get("/v1/me");
  expect(me.ok(), `reading the signed-in reader answered ${me.status()}`).toBe(
    true,
  );
  return readerFrom(await me.json());
}

// A seeded account has never walked the onboarding wizard, and the app shows
// the wizard instead of Home until its state says complete.
async function finishOnboarding(page: Page) {
  const state = await page.request.get("/v1/onboarding/state");
  if (state.ok()) return;
  expect(state.status(), "reading the onboarding state").toBe(404);
  const finished = await page.request.put("/v1/onboarding/state", {
    headers: { "Idempotency-Key": `bench-home-${Date.now()}` },
    data: {
      expected_version: 0,
      step: "complete",
      source_mode: null,
      company_draft: {},
      selected_fact_keys: [],
      voice_skipped: true,
      connect_skipped: true,
    },
  });
  expect(
    finished.ok(),
    `finishing onboarding answered ${finished.status()}`,
  ).toBe(true);
}

/** Opens a signed-in page whose first load has filled the browser cache. */
async function warmHome(
  page: Page,
): Promise<{ reader: Reader; log: AnswerLog }> {
  // The default buffer of 250 entries fills with the bundle's own files before
  // the worklist request is made.
  await page.addInitScript(() =>
    performance.setResourceTimingBufferSize(5_000),
  );
  const reader = await signIn(page);
  const log = new AnswerLog(page);
  await loadHome(page, log, reader, 0);
  return { reader, log };
}

/** Waits until mark journey `id` has set a final Home mark; false on timeout. */
async function settledMarks(page: Page, id: string): Promise<boolean> {
  const final = async () => {
    const marks = await journeyMarks(page, id, HOME_MARKS);
    return (
      marks["home:complete"] ?? marks["home:degraded"] ?? marks["home:failed"]
    );
  };
  return expect
    .poll(final, { timeout: JOURNEY_TIMEOUT_MS })
    .toBeDefined()
    .then(
      () => true,
      () => false,
    );
}

/**
 * The answers of `journey`, once every Home read has answered and no retry of
 * it is due. On a timeout the judgement names each read that never answered.
 */
async function answersOf(log: AnswerLog, journey: string): Promise<Answer[]> {
  await expect
    .poll(() => log.settled(journey, HOME_READS), {
      timeout: JOURNEY_TIMEOUT_MS,
    })
    .toBe(true)
    .catch(() => undefined);
  return log.close(journey);
}

/** Whether the day and the reader's greeting are on screen right now. */
async function screenReasons(page: Page, reader: Reader): Promise<string[]> {
  const reasons: string[] = [];
  if (!(await page.locator(DAY_ON_SCREEN).first().isVisible()))
    reasons.push("the day was not on screen");
  const glance = (await page.locator(GLANCE).textContent()) ?? "";
  if (reader.greeting !== null && !glance.includes(reader.greeting))
    reasons.push("the greeting did not name the reader");
  return reasons;
}

function judged(
  marks: Partial<Record<JourneyMark, number>>,
  settled: boolean,
  answers: Answer[],
  reasons: string[],
  from: number,
): Journey {
  if (marks["home:degraded"] !== undefined)
    reasons.push("the page marked itself degraded");
  const times: Record<string, number> = {};
  for (const [mark, at] of Object.entries(marks)) {
    times[mark] = Math.round(at - from);
  }
  const reached = {
    useful: marks["home:useful"] !== undefined,
    failed: marks["home:failed"] !== undefined,
  };
  return {
    outcome: outcomeOf(reached, settled, reasons),
    times,
    reasons,
    withheld: withheldSources(answers),
  };
}

/** One full load of Home, read back as the marks it set and what it got. */
async function loadHome(
  page: Page,
  log: AnswerLog,
  reader: Reader,
  sample: number,
): Promise<Journey> {
  const journey = `load-${sample}`;
  log.begin(journey);
  // A query that changes per sample forces a real document load; the same URL
  // again would be a same-document navigation that keeps the running app.
  await page.goto(`/?bench=${sample}`);
  const settled = await settledMarks(page, "");
  const answers = await answersOf(log, journey);
  const reasons = [
    ...degradedReasons(answers, EXPECTED, reader.id),
    ...(await screenReasons(page, reader)),
  ];
  const marks = await journeyMarks(page, "", HOME_MARKS);
  return judged(marks, settled, answers, reasons, 0);
}

/** From Home to Contacts and back through the rail, in the same document. */
async function returnHome(
  page: Page,
  log: AnswerLog,
  reader: Reader,
  sample: number,
): Promise<Journey> {
  const away = `away-${sample}`;
  log.begin(away);
  await awayLink(page).click();
  await expect(page.locator("#brief-today")).toHaveCount(0);
  const journey = `return-${sample}`;
  log.begin(journey);
  await log.close(away);
  const opened = await startJourney(page, journey);
  const screen = await watchScreen(page, DAY_ON_SCREEN, reader.greeting);
  const probe = await probeClick(page, HOME_LINK, DAY_ON_SCREEN);
  await homeLink(page).click();
  const settled = await settledMarks(page, journey);
  const answers = await answersOf(log, journey);
  const { clickAt, answeredAt } = await probe.evaluate((p) => p.result());
  const frames = await screen.evaluate((s) => s.stop());
  const reasons = degradedReasons(answers, EXPECTED, reader.id);
  const lost = framesLostAfterReturn(frames);
  if (lost.length > 0)
    reasons.push(`the day left the screen for ${lost.length} frames`);
  const marks = await journeyMarks(page, journey, HOME_MARKS);
  const result = judged(marks, settled, answers, reasons, clickAt ?? opened);
  // The screen's own answer beside the marks: the first frame showing the day.
  if (clickAt !== undefined && answeredAt !== undefined)
    result.times["day on screen"] = Math.round(answeredAt - clickAt);
  return result;
}

/**
 * The window regains focus, so Home's live reads refetch. The day's answer is
 * held until the queue button has answered a click.
 */
async function refreshHome(
  page: Page,
  log: AnswerLog,
  reader: Reader,
  sample: number,
): Promise<Journey> {
  const journey = `refresh-${sample}`;
  log.begin(journey);
  const opened = await startJourney(page, journey);
  const screen = await watchScreen(page, DAY_ON_SCREEN, reader.greeting);
  const probe = await probeClick(page, QUEUE_BUTTON, QUEUE_OPEN);
  const held = await holdWorklist(page);
  try {
    const refetch = page.waitForRequest(
      (request) =>
        readOf(new URL(request.url()), request.method()) === "worklist",
      { timeout: JOURNEY_TIMEOUT_MS },
    );
    await page.evaluate(() =>
      window.dispatchEvent(new Event("visibilitychange")),
    );
    await refetch;
    await queueButton(page, false).click();
    await expect(queueButton(page, true)).toBeVisible();
  } finally {
    await held.release();
  }
  const answers = await answersOf(log, journey);
  const { clickAt, answeredAt } = await probe.evaluate((p) => p.result());
  const frames = await screen.evaluate((s) => s.stop());
  const fetches = await worklistFetches(page, opened);
  // The open queue covers the button, so it closes the way a reader closes it.
  await page.keyboard.press("Escape");
  await expect(queueButton(page, false)).toBeVisible();
  const reasons = degradedReasons(answers, EXPECTED, reader.id);
  const gap = longestLoss(frames);
  if (frames.some((frame) => !frame.day || !frame.reader))
    reasons.push(`the day or its reader left the screen for ${gap} ms`);
  const during =
    clickAt !== undefined &&
    fetches.some((f) => f.start <= clickAt && clickAt <= f.end);
  if (!during) reasons.push("the click did not land during the refetch");
  const times: Record<string, number> = { "retained gap": gap };
  if (clickAt !== undefined && answeredAt !== undefined)
    times.interaction = Math.round(answeredAt - clickAt);
  return {
    outcome: reasons.length > 0 ? "degraded" : "complete",
    times,
    reasons,
    withheld: withheldSources(answers),
  };
}

function report(kind: string, journeys: readonly Journey[]) {
  const label = `bench-home: ${kind.padEnd(7)}`;
  const metrics = new Set(journeys.flatMap((j) => Object.keys(j.times)));
  for (const metric of metrics) {
    const times = journeys.flatMap((j) =>
      j.times[metric] === undefined ? [] : [j.times[metric]],
    );
    console.log(
      `${label} ${metric.padEnd(14)} p50 ${nearestRank(times, 0.5)} ms  p95 ${nearestRank(times, 0.95)} ms  over ${times.length}`,
    );
  }
  const { outcomes, reasons } = tally(journeys);
  console.log(
    `${label} ${[...outcomes].map(([o, n]) => `${n} ${o}`).join(", ")}`,
  );
  for (const [reason, count] of reasons)
    console.log(`${label} ${count}× ${reason}`);
  const withheld = new Set(journeys.flatMap((j) => j.withheld));
  if (withheld.size > 0)
    console.log(
      `${label} withheld by grant, not degraded: ${[...withheld].join(", ")}`,
    );
}

function p95(journeys: readonly Journey[], metric: string): number {
  return nearestRank(budgetSamples(journeys, metric), 0.95);
}

function expectAllComplete(kind: string, journeys: readonly Journey[]) {
  const unhealthy = journeys.filter((j) => j.outcome !== "complete");
  expect(
    unhealthy.map((j) => `${j.outcome}: ${j.reasons.join("; ")}`),
    `${kind} journeys that were not complete`,
  ).toEqual([]);
}

test("load: Home becomes useful and complete within R01", async ({ page }) => {
  const { reader, log } = await warmHome(page);
  const loads: Journey[] = [];
  for (let i = 1; i <= SAMPLES; i++) {
    loads.push(await loadHome(page, log, reader, i));
  }
  report("load", loads);
  // The last load's api requests, so a slow mark names the request it waited on.
  const requests = await page.evaluate(() =>
    performance
      .getEntriesByType("resource")
      .filter((entry) => new URL(entry.name).pathname.startsWith("/v1/"))
      .map((entry) => ({
        path: new URL(entry.name).pathname,
        from: Math.round(entry.startTime),
        to: Math.round(entry.startTime + entry.duration),
      }))
      .sort((a, b) => a.from - b.from),
  );
  for (const request of requests) {
    console.log(
      `bench-home: request ${String(request.from).padStart(5)} → ${String(request.to).padStart(5)} ms  ${request.path}`,
    );
  }
  expectAllComplete("load", loads);
  for (const [mark, budget] of Object.entries(LOAD_BUDGET_MS)) {
    expect(p95(loads, mark), `${mark} p95`).toBeLessThanOrEqual(budget);
  }
});

test("return: Home is useful again within 200 ms and keeps its day", async ({
  page,
}) => {
  const { reader, log } = await warmHome(page);
  const returns: Journey[] = [];
  for (let i = 1; i <= SAMPLES; i++) {
    returns.push(await returnHome(page, log, reader, i));
  }
  report("return", returns);
  expectAllComplete("return", returns);
  for (const metric of ["home:useful", "day on screen"]) {
    expect(p95(returns, metric), `${metric} again p95`).toBeLessThanOrEqual(
      RETURN_BUDGET_MS,
    );
  }
});

test("refresh: the day stays for its reader and the queue answers within 200 ms", async ({
  page,
}) => {
  const { reader, log } = await warmHome(page);
  const refreshes: Journey[] = [];
  for (let i = 1; i <= SAMPLES; i++) {
    refreshes.push(await refreshHome(page, log, reader, i));
  }
  report("refresh", refreshes);
  expectAllComplete("refresh", refreshes);
  expect(
    p95(refreshes, "interaction"),
    "interaction during refresh p95",
  ).toBeLessThanOrEqual(INTERACTION_BUDGET_MS);
});
