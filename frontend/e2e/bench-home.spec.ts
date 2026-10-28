import { expect, type Page, test } from "@playwright/test";
import { type JourneyMark, journeyMarkName } from "../src/lib/journeymarks";

/**
 * R01: the first useful Home within 2 s at p95, and its primary sections within
 * 2.5 s, on a production build against a live api (`make bench-home`).
 *
 * Each sample is a full page load of Home in a signed-in browser: the app
 * starts cold, the browser cache stays warm. The times are the app's own
 * journey marks (src/lib/journeymarks.ts), measured from navigation start.
 * A failed or timed-out load is counted as such, never as a latency.
 */

const SAMPLES = Number(process.env.BENCH_HOME_SAMPLES ?? 20);
const BUDGET_MS = { "home:useful": 2_000, "home:complete": 2_500 };
const MEASURED: JourneyMark[] = [
  "session",
  "gates",
  "home:useful",
  "home:complete",
];
const LOAD_TIMEOUT_MS = 15_000;

type Sample = Partial<Record<JourneyMark, number>>;

/** Nearest-rank quantile, the method perf-mobile.spec.ts and the Go harness use. */
function nearestRank(samples: number[], quantile: number): number {
  const sorted = [...samples].sort((a, b) => a - b);
  const rank = Math.ceil(sorted.length * quantile) - 1;
  return sorted[Math.min(Math.max(rank, 0), sorted.length - 1)];
}

async function signIn(page: Page) {
  const response = await page.request.post("/v1/auth/login", {
    data: {
      email: process.env.BENCH_HOME_EMAIL ?? "admin@demo.test",
      password: process.env.BENCH_HOME_PASSWORD ?? "demo-password-123",
    },
  });
  expect(response.ok(), `sign-in answered ${response.status()}`).toBe(true);
  // A seeded account has never walked the onboarding wizard, and the app shows
  // the wizard instead of Home until its state says complete.
  const state = await page.request.get("/v1/onboarding/state");
  if (state.status() !== 404) return;
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
  expect(finished.ok(), `finishing onboarding answered ${finished.status()}`).toBe(true);
}

/** One full load of Home, read back as the marks it set. */
async function loadHome(page: Page, sample: number): Promise<Sample> {
  // A query that changes per sample forces a real document load; the same URL
  // again would be a same-document navigation that keeps the running app.
  await page.goto(`/?bench=${sample}`);
  const names = Object.fromEntries(
    [...MEASURED, "home:failed" as const].map((mark) => [
      mark,
      journeyMarkName(mark),
    ]),
  );
  await page.waitForFunction(
    ({ complete, failed }) =>
      performance.getEntriesByName(complete, "mark").length > 0 ||
      performance.getEntriesByName(failed, "mark").length > 0,
    { complete: names["home:complete"], failed: names["home:failed"] },
    { timeout: LOAD_TIMEOUT_MS },
  );
  return page.evaluate((byMark) => {
    const out: Record<string, number> = {};
    for (const [mark, name] of Object.entries(byMark)) {
      const entry = performance.getEntriesByName(name, "mark")[0];
      if (entry) out[mark] = Math.round(entry.startTime);
    }
    return out;
  }, names);
}

test("Home becomes useful and complete within R01", async ({ page }) => {
  // The default buffer of 250 entries fills with the bundle's own files before
  // the worklist request is made.
  await page.addInitScript(() => performance.setResourceTimingBufferSize(5_000));
  await signIn(page);
  // The first load fills the browser cache; it is not a sample.
  await loadHome(page, 0);
  const samples: Sample[] = [];
  let failed = 0;
  let timedOut = 0;
  for (let i = 1; i <= SAMPLES; i++) {
    try {
      const sample = await loadHome(page, i);
      if (sample["home:failed"] === undefined) samples.push(sample);
      else failed++;
    } catch {
      timedOut++;
    }
  }
  for (const mark of MEASURED) {
    const times = samples.flatMap((s) => (s[mark] === undefined ? [] : [s[mark]]));
    if (times.length === 0) continue;
    console.log(
      `bench-home: ${mark.padEnd(14)} p50 ${nearestRank(times, 0.5)} ms  p95 ${nearestRank(times, 0.95)} ms  over ${times.length}`,
    );
  }
  console.log(`bench-home: ${samples.length} loaded, ${failed} failed, ${timedOut} timed out`);
  // The last load's api requests, so a slow mark names the request it waited on.
  const requests = await page.evaluate(() =>
    performance
      .getEntriesByType("resource")
      .filter((entry) => new URL(entry.name).pathname.startsWith("/v1/"))
      .map((entry) => ({
        path: new URL(entry.name).pathname,
        from: Math.round(entry.startTime),
        to: Math.round((entry as PerformanceResourceTiming).responseEnd),
      }))
      .sort((a, b) => a.from - b.from),
  );
  for (const request of requests) {
    console.log(`bench-home: request ${String(request.from).padStart(5)} → ${String(request.to).padStart(5)} ms  ${request.path}`);
  }
  expect(failed, "Home loads that rendered the failed state").toBe(0);
  expect(timedOut, `Home loads that set no mark within ${LOAD_TIMEOUT_MS} ms`).toBe(0);
  for (const [mark, budget] of Object.entries(BUDGET_MS)) {
    const times = samples.map((s) => s[mark as JourneyMark] ?? Number.POSITIVE_INFINITY);
    expect(nearestRank(times, 0.95), `${mark} p95`).toBeLessThanOrEqual(budget);
  }
});
