import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, type Page, type Request, test } from "@playwright/test";
import { mockApi } from "./seed";

/**
 * MOBILE-AC-2: record open p95 < 300 ms perceived, on a throttled Fast-3G
 * profile at 390px. The BUDGET is PERF-1's and is single-homed there;
 * MOBILE-PARAM-2 pins only the condition it has to hold under.
 *
 * This is the ONLY place PERF-1's perceived budget is asserted. The acceptance
 * lane keeps the structural claim — the head renders from the route, not from
 * the read — and no number, because one wall-clock sample on a runner shared
 * with six integration shards measures the machine. Throttled p95 is the harder
 * condition: a budget that holds on Fast-3G holds on a fast link by
 * construction, so an unthrottled copy would add a second answer, not a second
 * question.
 *
 * Run it with `make bench-mobile`. It is not collected by `pnpm e2e`.
 */

declare global {
  interface Window {
    /** Long tasks the renderer ran, stamped on the wall clock so a window
     * measured from the driver's side can be compared against them. */
    perfLongTasks?: LongTask[];
  }
}

// Chrome DevTools' own Fast-3G preset. Named constants rather than inline
// arithmetic because these are the numbers MOBILE-PARAM-2 refers to by name,
// and a reader has to be able to check them against the profile they claim.
const FAST_3G = {
  downloadThroughput: (1.6 * 1024 * 1024) / 8, // 1.6 Mbit/s
  uploadThroughput: (750 * 1024) / 8, // 750 kbit/s
  latency: 562.5, // ms round trip
};

const SAMPLES = 20;
const PERCEIVED_BUDGET_MS = 300;

/** One measured open: the click, the heading, and what sat between them. */
type Open = { from: number; to: number; ms: number };

/** One request's LIFE on the wire, rather than the moment it was issued. */
type Exchange = { path: string; from: number; to: number };

/** One task that held the renderer's main thread for over 50 ms. */
type LongTask = { at: number; ms: number };

/**
 * Throttle the link the way a phone experiences it.
 *
 * TWO mechanisms, because either alone would measure a lie here. CDP throttling
 * shapes real network traffic — the bundle, the fonts — but the seed fixture is
 * mocked at the network edge, and a fulfilled route never touches the transport
 * CDP is shaping. So the API's round-trip cost has to be paid explicitly.
 *
 * The delay route is registered AFTER mockApi so it matches FIRST (Playwright
 * runs handlers in reverse registration order), waits out one round trip, and
 * then falls back to the seed mock rather than answering in its place.
 */
async function throttle(page: Page) {
  const session = await page.context().newCDPSession(page);
  await session.send("Network.enable");
  await session.send("Network.emulateNetworkConditions", {
    offline: false,
    ...FAST_3G,
  });
  await page.route("**/v1/**", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, FAST_3G.latency));
    await route.fallback();
  });
}

/** Nearest-rank quantile, the same method the Go harness reports — so a value
 * here is a latency that actually happened rather than an interpolation between
 * two that did not. One spelling for every quantile this file reports: the
 * published record used to inline the same arithmetic twice more, and three
 * copies of a rank formula are three chances to be off by one. */
function nearestRank(samples: number[], quantile: number): number {
  const sorted = [...samples].sort((a, b) => a - b);
  const rank = Math.ceil(sorted.length * quantile) - 1;
  return sorted[Math.min(Math.max(rank, 0), sorted.length - 1)];
}

/**
 * Every request's SPAN, over every path.
 *
 * The instrument this replaces recorded the moment a read was ISSUED, and only
 * `/v1/` paths. It answered "no reads" for all six slow opens of the last run,
 * and both of its limits hide the same thing, so that answer settles nothing:
 *
 * - A `/v1/` handler here sleeps out a 562 ms round trip — LONGER than any open
 *   this lane measures. A refetch issued shortly before a click is in flight for
 *   the whole of the open while STARTING outside it, and a start-time filter
 *   reports zero for exactly the case it was built to catch.
 * - A lazily fetched route chunk is not a `/v1/` path at all, and at 1.6 Mbit/s
 *   a ~40 KB chunk costs about the 220 ms that separates this run's two modes.
 *
 * What a window needs is traffic OVERLAPPING it. A request that never finishes
 * keeps an infinite `to`, which is the honest reading: still in flight.
 */
function recordTraffic(page: Page): Exchange[] {
  const exchanges: Exchange[] = [];
  const inFlight = new Map<Request, Exchange>();
  page.on("request", (request) => {
    const exchange = {
      // Paths only: a full URL adds the origin to every line and answers
      // nothing this is asking.
      path: new URL(request.url()).pathname,
      from: Date.now(),
      to: Number.POSITIVE_INFINITY,
    };
    inFlight.set(request, exchange);
    exchanges.push(exchange);
  });
  const settle = (request: Request) => {
    const exchange = inFlight.get(request);
    if (exchange) exchange.to = Date.now();
    inFlight.delete(request);
  };
  page.on("requestfinished", settle);
  page.on("requestfailed", settle);
  return exchanges;
}

/**
 * Watch the renderer's main thread, which is the half of a slow open that no
 * amount of request logging can see.
 *
 * Traffic and main-thread work are the two things an open can be WAITING for,
 * and the pair is what makes either reading conclusive. A slow open with a long
 * task ran code — that is a real cost a user meets, and it is bisectable. A slow
 * open with neither traffic nor a long task did not wait for the product at all,
 * and the stall is in the harness measuring it.
 *
 * Stamped on the wall clock (`timeOrigin` + `startTime`) because the window it
 * has to be compared against is measured from the driver's side in `Date.now()`.
 * Drained ONCE at the end rather than per sample: a round trip between samples
 * would change the pace of the very loop whose pace is under investigation.
 */
async function observeLongTasks(page: Page) {
  await page.addInitScript(() => {
    window.perfLongTasks = [];
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        window.perfLongTasks?.push({
          at: Math.round(performance.timeOrigin + entry.startTime),
          ms: Math.round(entry.duration),
        });
      }
    }).observe({ type: "longtask", buffered: true });
  });
}

/** Whether something running from `from` to `to` was in flight during the open.
 * Strict at both ends: a request that settled ON the click is not what the click
 * waited for, and one issued as the heading appeared is not either. */
function overlaps(open: Open, from: number, to: number): boolean {
  return from < open.to && to > open.from;
}

/**
 * Open one record from the list, and answer what the click cost.
 *
 * Extracted so the discarded first open below runs the SAME path as a measured
 * one. A warm-up that took a shortcut would leave whatever it skipped to be
 * paid by sample 1, which is the defect it exists to remove.
 */
async function openOneRecord(page: Page): Promise<Open> {
  await page.goto("/#/contacts");
  // Anchor on a settled screen before measuring, for the reason ac.spec.ts
  // records: a click during hydration lands on a row whose handler is not
  // attached, the navigation never happens, and the assertion times out as a
  // phantom perf failure. Under throttling this is likelier, not less likely.
  await page.waitForLoadState("networkidle");
  // By ROLE, for the reason ac.spec.ts records: a record's name is asserted
  // through the element the view actually draws — here a table row — so the
  // locator still names one thing when another surface repeats the name. The
  // row stays a substring match, because a row's accessible name is every cell
  // of it joined and the person's name is a fragment of that by construction.
  const row = page.getByRole("row", { name: "Anna Weber" });
  await expect(row).toBeVisible();

  const from = Date.now();
  await row.click();
  // The record's OWN header, not the shell's: the head shows only the trail
  // on a record route and renders from the router before any record read
  // returns, so waiting on it would measure routing rather than the open.
  // Exact: the whole name is what says the right record opened, and `name`
  // matches by substring without it.
  await expect(
    page.getByRole("heading", { level: 1, name: "Anna Weber", exact: true }),
  ).toBeVisible();
  // Both ENDS are returned rather than left to be derived. A caller computing
  // the start as `end - ms` from its own clock reads LATER than this one did,
  // because the await returns before that line runs — and the traffic it would
  // drop is what the click issued in its first milliseconds, which is precisely
  // what the window is being measured to catch.
  const to = Date.now();
  return { from, to, ms: to - from };
}

/**
 * Say what the run measured, in the four readings it takes to tell this lane's
 * breaches apart. Whoever reads a breach reads this log and nothing else.
 */
function report(
  opens: Open[],
  traffic: Exchange[],
  longTasks: LongTask[],
  measured: number,
) {
  const samples = opens.map((open) => open.ms);
  console.log(
    `perfbench [fast-3g/390px]: record_open_perceived p95=${measured}ms ` +
      `(budget ${PERCEIVED_BUDGET_MS}ms, ${SAMPLES} samples)`,
  );
  // The SHAPE as well as the verdict, because this lane can report breaches
  // that need opposite answers and a lone p95 does not separate them: a
  // distribution that has moved is a regression to bisect, a tight one behind a
  // single straggler is a runner that was busy.
  console.log(
    `perfbench [fast-3g/390px]: record_open_perceived ` +
      `p50=${nearestRank(samples, 0.5)}ms p99=${nearestRank(samples, 0.99)}ms ` +
      `min=${Math.min(...samples)}ms max=${Math.max(...samples)}ms ` +
      `samples=${JSON.stringify([...samples].sort((a, b) => a - b))}`,
  );
  // And in the order they were MEASURED, which is the one question the sorted
  // line cannot answer. A third shape reaches this lane: a tight fast body with
  // several stragglers behind a clean gap, which is neither a moved
  // distribution nor one unlucky sample. Sorting throws exactly that away.
  //
  // `span_ms` is the wall clock the whole loop took, and it is what makes a
  // laptop's run comparable to a runner's at all. The two are not the same
  // experiment: this loop has run in 1.3s on a developer machine and in ~115s
  // on CI, and the difference decides whether the twenty opens fit inside ONE
  // 30s `STALE_TIME_MS` window (so nothing is ever re-read) or cross four.
  console.log(
    `perfbench [fast-3g/390px]: record_open_perceived ` +
      `span_ms=${opens[opens.length - 1].to - opens[0].from} ` +
      `in_order=${JSON.stringify(samples)}`,
  );
  // The two things an open can WAIT for, aligned index-for-index with
  // `in_order` above: requests in flight across the window, and milliseconds
  // the renderer's main thread spent blocked inside it. A slow open with
  // neither waited for nothing the product did.
  const busy = opens.map((open) => ({
    paths: traffic
      .filter((exchange) => overlaps(open, exchange.from, exchange.to))
      .map((exchange) => exchange.path),
    blockedMs: longTasks
      .filter((task) => overlaps(open, task.at, task.at + task.ms))
      .reduce((total, task) => total + task.ms, 0),
  }));
  console.log(
    `perfbench [fast-3g/390px]: record_open_perceived ` +
      `in_flight=${JSON.stringify(busy.map((b) => b.paths.length))} ` +
      `blocked_ms=${JSON.stringify(busy.map((b) => b.blockedMs))}`,
  );
  // And WHICH requests, for the windows that had any — the counts say something
  // was in flight, the paths say what, and a fix has to name an endpoint or a
  // chunk rather than a number.
  busy.forEach((b, i) => {
    if (b.paths.length > 0 || b.blockedMs > 0) {
      console.log(
        `perfbench [fast-3g/390px]: record_open_perceived ` +
          `sample=${i} ms=${samples[i]} blocked_ms=${b.blockedMs} ` +
          `in_flight=${JSON.stringify(b.paths)}`,
      );
    }
  });
}

test("MOBILE-AC-2: record open holds the 300ms perceived budget on Fast-3G at 390px", async ({
  page,
}) => {
  await mockApi(page);
  await throttle(page);
  const traffic = recordTraffic(page);
  await observeLongTasks(page);

  // ONE DISCARDED OPEN FIRST, and it is not a kindness to the number.
  //
  // The first open in a session pays for what every later one finds already
  // there — the record route's code chunk, and its first data over a link
  // throttled to Fast-3G. That cost is real and a user meets it, but it is the
  // cost of ARRIVING, paid once per session; MOBILE-AC-2 is the cost of opening
  // a record, which a user pays all day. One number over both measures neither,
  // and the gap is not a rounding error: the cold open runs to roughly twenty
  // times a warm one.
  //
  // Which matters here more than the average of it, because of where p95 lands.
  // Nearest rank over 20 samples is the SECOND-HIGHEST of them, so a single
  // guaranteed outlier rests the whole assertion on the worst remaining sample
  // and leaves the lane one unlucky sample from red — a budget with sixfold
  // room to spare, breaching on a busy runner while nothing about opening a
  // record has changed. Discarding the arrival is what buys the headroom back.
  await openOneRecord(page);

  const opens: Open[] = [];
  for (let i = 0; i < SAMPLES; i++) {
    opens.push(await openOneRecord(page));
  }
  const longTasks = await page.evaluate(() => window.perfLongTasks ?? []);

  // The INSTRUMENT's liveness, before any reading is taken from it. `blocked_ms`
  // answers its question by reporting zero, and an observer that silently
  // stopped delivering reports zero too — under-recognition is the one way this
  // must not break, because it reads as a clean answer with no failing
  // assertion to notice. The discarded arrival parses the bundle over a Fast-3G
  // link and blocks the main thread far past the 50ms long-task threshold every
  // time, so a run that saw NO long task anywhere saw nothing rather than
  // nothing happening. This lane has already retired one hypothesis on a
  // filter's silence; it does not get to do that twice.
  expect(
    longTasks.length,
    "the long-task observer delivered nothing, so blocked_ms means nothing",
  ).toBeGreaterThan(0);

  const samples = opens.map((open) => open.ms);
  const measured = nearestRank(samples, 0.95);
  report(opens, traffic, longTasks, measured);
  // Written BEFORE the assertion, deliberately: a breach is the run whose
  // number a reader most wants to see, and recording afterwards would leave
  // the published page green while the run went red.
  if (recordingEnabled()) {
    writeRecord(samples, measured);
  }
  expect(measured).toBeLessThan(PERCEIVED_BUDGET_MS);
});

/**
 * Whether this run publishes its numbers, read from the same variable the Go
 * bench suites read (`RecordingEnabled`, backend/internal/compose/integration/
 * perfrecord.go). One spelling across both languages, because the rule is one
 * rule: a record is a human's to publish, and a scheduled job is exactly the
 * machine that must not write one.
 *
 * `=== "1"` rather than "set and non-empty", so a target can CLEAR the variable
 * to mean it — `MARGINCE_BENCH_RECORD=` in a check target has to beat whatever
 * an earlier by-hand run left exported in the caller's shell.
 */
function recordingEnabled(): boolean {
  return process.env.MARGINCE_BENCH_RECORD === "1";
}

/**
 * Leave this run's numbers where the published page reads them.
 *
 * Deliberately the same JSON shape the Go bench suites write
 * (backend/internal/compose/integration/perfrecord.go), so one renderer serves
 * all three and a fourth measurement in a fourth language needs no new reader.
 *
 * What is NOT recorded: hostname, username, and any filesystem path. This lands
 * in a public repository, and none of the three tells a reader why a number is
 * what it is.
 */
function writeRecord(samples: number[], measured: number) {
  const record = {
    target: "bench-mobile",
    // The DAY, not the instant: a record that changed every run would churn the
    // committed page for no reader's benefit.
    measured_on: new Date().toISOString().slice(0, 10),
    machine: {
      os: os.platform(),
      arch: os.arch(),
      cpu: os.cpus()[0]?.model.trim() ?? "unknown",
      cores: os.cpus().length,
      memory_gib: Math.round(os.totalmem() / 1024 ** 3),
      toolchain: `node ${process.versions.node}`,
      // The condition the budget must hold under is part of what was measured,
      // not a footnote — a 300ms p95 on a fast link is a different claim.
      network: "throttled Fast-3G (MOBILE-PARAM-2)",
      viewport: "390x844",
    },
    budgets: [
      {
        id: "MOBILE-AC-2",
        name: "record_open_perceived",
        p50_ms: nearestRank(samples, 0.5),
        p95_ms: measured,
        p99_ms: nearestRank(samples, 0.99),
        budget_ms: PERCEIVED_BUDGET_MS,
        samples: samples.length,
      },
    ],
  };
  const dir = path.join("..", "docs", "reference", "perfbench");
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(
    path.join(dir, "bench-mobile.json"),
    `${JSON.stringify(record, null, 2)}\n`,
  );
}
