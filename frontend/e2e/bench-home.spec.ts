import { expect, type Page, test } from "@playwright/test";
import type { JourneyMark } from "../src/lib/journeymarks";
import {
  type Answer,
  AnswerLog,
  degradedReasons,
  expectationFrom,
  type Frame,
  HOME_READS,
  journeyMarks,
  probeClick,
  readerFrom,
  startJourney,
  watchScreen,
  withheldSources,
  worklistFetches,
} from "./homejourney";
import { nearestRank } from "./quantile";

/**
 * R01 and R07 on a production build against a live api (`make bench-home`).
 *
 * Three journeys, each repeated SAMPLES times:
 *  - load: a full page load of Home in a signed-in browser. The app starts
 *    cold, the browser cache stays warm. Budgets: useful p95 2 s, complete
 *    p95 2.5 s.
 *  - return: from Home to Contacts and back through the rail, in one
 *    document. Budget: useful again p95 200 ms, and the day never leaves the
 *    screen once it is back.
 *  - refresh: the window regains focus, so the live reads refetch. The day
 *    stays on screen for the same reader throughout, and the queue button
 *    answers a click made during the refetch within 200 ms at p95.
 *
 * The times are the app's own journey marks (src/lib/journeymarks.ts). Every
 * journey also judges the answers the page received (homejourney.ts), so a
 * load whose sections answered but held a failed source, or an empty day for a
 * reader who has work, is counted as degraded and never as complete. A failed
 * or timed-out journey is counted as such, never as a latency.
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

// What "the day is on screen" means for this reader: the lead card of a day
// with work, or the clear-day plate of a reader who has none.
const DAY_ON_SCREEN = EXPECTED.work
  ? "#brief-today .brief-triage-lead"
  : "#brief-today .surfacestate-empty";
const QUEUE_BUTTON = "#brief-today .record-details-toggle";
const AWAY_LINK = 'nav a.navitem[href="#/contacts"]';
const HOME_LINK = 'nav a.navitem[href="#/home"]';

type Outcome = "complete" | "degraded" | "useful" | "failed" | "timed out";

type Journey = {
  outcome: Outcome;
  times: Record<string, number>;
  reasons: string[];
  withheld: string[];
};

type Reader = { id: string; greeting: string | null };

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

/** Waits until journey `id` has set a final Home mark; false on a timeout. */
async function settledMarks(page: Page, id: string): Promise<boolean> {
  try {
    await expect
      .poll(
        async () => {
          const marks = await journeyMarks(page, id, HOME_MARKS);
          return (
            marks["home:complete"] ??
            marks["home:degraded"] ??
            marks["home:failed"]
          );
        },
        { timeout: JOURNEY_TIMEOUT_MS },
      )
      .toBeDefined();
    return true;
  } catch {
    return false;
  }
}

/** Waits for every Home read to answer; the judgement names a missing one. */
async function answersOf(log: AnswerLog): Promise<Answer[]> {
  try {
    await expect
      .poll(() => log.holds(HOME_READS), { timeout: JOURNEY_TIMEOUT_MS })
      .toBe(true);
  } catch {
    // Judged below: degradedReasons names each read that never answered.
  }
  return log.take();
}

function judged(
  marks: Partial<Record<JourneyMark, number>>,
  settled: boolean,
  answers: Answer[],
  reader: Reader,
  from: number,
): Journey {
  const reasons = degradedReasons(answers, EXPECTED, reader.id);
  if (marks["home:degraded"] !== undefined)
    reasons.push("the page marked itself degraded");
  const times: Record<string, number> = {};
  for (const [mark, at] of Object.entries(marks)) {
    times[mark] = Math.round(at - from);
  }
  return {
    outcome: outcomeOf(marks, settled, reasons),
    times,
    reasons,
    withheld: withheldSources(answers),
  };
}

function outcomeOf(
  marks: Partial<Record<JourneyMark, number>>,
  settled: boolean,
  reasons: readonly string[],
): Outcome {
  if (marks["home:failed"] !== undefined) return "failed";
  if (!settled)
    return marks["home:useful"] === undefined ? "timed out" : "useful";
  return reasons.length > 0 ? "degraded" : "complete";
}

/** One full load of Home, read back as the marks it set and what it got. */
async function loadHome(
  page: Page,
  log: AnswerLog,
  reader: Reader,
  sample: number,
): Promise<Journey> {
  await quiet(log);
  // A query that changes per sample forces a real document load; the same URL
  // again would be a same-document navigation that keeps the running app.
  await page.goto(`/?bench=${sample}`);
  const settled = await settledMarks(page, "");
  const answers = await answersOf(log);
  return judged(
    await journeyMarks(page, "", HOME_MARKS),
    settled,
    answers,
    reader,
    0,
  );
}

/** Waits out the Home reads on the wire, then drops what they answered. */
async function quiet(log: AnswerLog) {
  await expect
    .poll(() => log.inFlight(), { timeout: JOURNEY_TIMEOUT_MS })
    .toBe(0);
  await log.take();
}

/** The longest stretch, in ms, of frames that lost the day or its reader. */
function longestLoss(frames: readonly Frame[]): number {
  let longest = 0;
  let lostSince: number | undefined;
  for (const frame of frames) {
    if (!frame.day || !frame.reader) lostSince ??= frame.at;
    else lostSince = undefined;
    if (lostSince !== undefined)
      longest = Math.max(longest, frame.at - lostSince);
  }
  return Math.round(longest);
}

/** Frames that lost the day or its reader after it first came back. */
function framesLost(frames: readonly Frame[]): Frame[] {
  const back = frames.findIndex((frame) => frame.day && frame.reader);
  if (back < 0) return [...frames];
  return frames.slice(back).filter((frame) => !frame.day || !frame.reader);
}

/** From Home to Contacts and back through the rail, in the same document. */
async function returnHome(
  page: Page,
  log: AnswerLog,
  reader: Reader,
  sample: number,
): Promise<Journey> {
  await page.locator(AWAY_LINK).click();
  await expect(page.locator("#brief-today")).toHaveCount(0);
  await quiet(log);
  const id = `return-${sample}`;
  const opened = await startJourney(page, id);
  const screen = await watchScreen(page, DAY_ON_SCREEN, reader.greeting);
  const probe = await probeClick(page, HOME_LINK, DAY_ON_SCREEN);
  await page.locator(HOME_LINK).click();
  const settled = await settledMarks(page, id);
  const answers = await answersOf(log);
  const { clickAt, answeredAt } = await probe.evaluate((p) => p.result());
  const frames = await screen.evaluate((s) => s.stop());
  const journey = judged(
    await journeyMarks(page, id, HOME_MARKS),
    settled,
    answers,
    reader,
    clickAt ?? opened,
  );
  // The screen's own answer beside the marks: the first frame showing the day.
  if (clickAt !== undefined && answeredAt !== undefined)
    journey.times["day on screen"] = Math.round(answeredAt - clickAt);
  const lost = framesLost(frames);
  if (lost.length > 0)
    journey.reasons.push(`the day left the screen for ${lost.length} frames`);
  if (lost.length > 0 && journey.outcome === "complete")
    journey.outcome = "degraded";
  return journey;
}

/**
 * The window regains focus, so Home's live reads refetch; the queue button is
 * clicked while the day's refetch is in flight.
 */
async function refreshHome(
  page: Page,
  log: AnswerLog,
  reader: Reader,
  sample: number,
): Promise<Journey & { overlapped: boolean }> {
  await quiet(log);
  const opened = await startJourney(page, `refresh-${sample}`);
  const screen = await watchScreen(page, DAY_ON_SCREEN, reader.greeting);
  const pressed = `${QUEUE_BUTTON}[aria-pressed="true"]`;
  const probe = await probeClick(page, QUEUE_BUTTON, pressed);
  const refetch = page.waitForRequest(
    (request) =>
      new URL(request.url()).pathname === "/v1/worklist" &&
      request.method() === "GET",
    { timeout: JOURNEY_TIMEOUT_MS },
  );
  await page.evaluate(() =>
    window.dispatchEvent(new Event("visibilitychange")),
  );
  await refetch;
  await page.locator(QUEUE_BUTTON).click();
  await expect(page.locator(pressed)).toHaveCount(1);
  const answers = await answersOf(log);
  const { clickAt, answeredAt } = await probe.evaluate((p) => p.result());
  const frames = await screen.evaluate((s) => s.stop());
  const fetches = await worklistFetches(page, opened);
  // The open queue covers the button, so it closes the way a reader closes it.
  await page.keyboard.press("Escape");
  await expect(page.locator(pressed)).toHaveCount(0);
  const reasons = degradedReasons(answers, EXPECTED, reader.id);
  const gap = longestLoss(frames);
  if (frames.some((frame) => !frame.day || !frame.reader))
    reasons.push(`the day or its reader left the screen for ${gap} ms`);
  const times: Record<string, number> = { "retained gap": gap };
  if (clickAt !== undefined && answeredAt !== undefined)
    times.interaction = Math.round(answeredAt - clickAt);
  return {
    outcome: reasons.length > 0 ? "degraded" : "complete",
    times,
    reasons,
    withheld: withheldSources(answers),
    overlapped:
      clickAt !== undefined &&
      fetches.some((f) => f.start <= clickAt && clickAt <= f.end),
  };
}

function report(kind: string, journeys: readonly Journey[]) {
  const metrics = new Set(journeys.flatMap((j) => Object.keys(j.times)));
  for (const metric of metrics) {
    const times = journeys.flatMap((j) =>
      j.times[metric] === undefined ? [] : [j.times[metric]],
    );
    console.log(
      `bench-home: ${kind.padEnd(7)} ${metric.padEnd(14)} p50 ${nearestRank(times, 0.5)} ms  p95 ${nearestRank(times, 0.95)} ms  over ${times.length}`,
    );
  }
  const outcomes = new Map<Outcome, number>();
  const reasons = new Map<string, number>();
  for (const journey of journeys) {
    outcomes.set(journey.outcome, (outcomes.get(journey.outcome) ?? 0) + 1);
    for (const reason of journey.reasons)
      reasons.set(reason, (reasons.get(reason) ?? 0) + 1);
  }
  console.log(
    `bench-home: ${kind.padEnd(7)} ${[...outcomes].map(([o, n]) => `${n} ${o}`).join(", ")}`,
  );
  for (const [reason, count] of reasons)
    console.log(`bench-home: ${kind.padEnd(7)} ${count}× ${reason}`);
  const withheld = new Set(journeys.flatMap((j) => j.withheld));
  if (withheld.size > 0)
    console.log(
      `bench-home: ${kind.padEnd(7)} withheld by grant, not degraded: ${[...withheld].join(", ")}`,
    );
}

/** The p95 of `metric`, with a journey that did not reach it counted as never. */
function p95(journeys: readonly Journey[], metric: string): number {
  return nearestRank(
    journeys.map((j) =>
      j.outcome === "complete"
        ? (j.times[metric] ?? Number.POSITIVE_INFINITY)
        : Number.POSITIVE_INFINITY,
    ),
    0.95,
  );
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
  const refreshes: (Journey & { overlapped: boolean })[] = [];
  for (let i = 1; i <= SAMPLES; i++) {
    refreshes.push(await refreshHome(page, log, reader, i));
  }
  report("refresh", refreshes);
  const during = refreshes.filter((r) => r.overlapped);
  console.log(
    `bench-home: refresh clicks made while the day was refetching: ${during.length} of ${refreshes.length}`,
  );
  expectAllComplete("refresh", refreshes);
  expect(
    during.length,
    "clicks that landed while the day was refetching; with none, no interaction during a refresh was measured",
  ).toBeGreaterThan(0);
  expect(
    p95(during, "interaction"),
    "interaction during refresh p95",
  ).toBeLessThanOrEqual(INTERACTION_BUDGET_MS);
});
