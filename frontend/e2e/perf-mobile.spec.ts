import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, type Page, test } from "@playwright/test";
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
 * Open one record from the list, and answer what the click cost.
 *
 * Extracted so the discarded first open below runs the SAME path as a measured
 * one. A warm-up that took a shortcut would leave whatever it skipped to be
 * paid by sample 1, which is the defect it exists to remove.
 */
async function openOneRecord(page: Page): Promise<number> {
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

  const start = Date.now();
  await row.click();
  // The record's OWN header, not the shell's: the head shows only the trail
  // on a record route and renders from the router before any record read
  // returns, so waiting on it would measure routing rather than the open.
  // Exact: the whole name is what says the right record opened, and `name`
  // matches by substring without it.
  await expect(
    page.getByRole("heading", { level: 1, name: "Anna Weber", exact: true }),
  ).toBeVisible();
  return Date.now() - start;
}

test("MOBILE-AC-2: record open holds the 300ms perceived budget on Fast-3G at 390px", async ({
  page,
}) => {
  await mockApi(page);
  await throttle(page);

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

  const samples: number[] = [];
  for (let i = 0; i < SAMPLES; i++) {
    samples.push(await openOneRecord(page));
  }

  const measured = nearestRank(samples, 0.95);
  console.log(
    `perfbench [fast-3g/390px]: record_open_perceived p95=${measured}ms ` +
      `(budget ${PERCEIVED_BUDGET_MS}ms, ${SAMPLES} samples)`,
  );
  // The SHAPE as well as the verdict, because this lane can report two breaches
  // that need opposite answers and a lone p95 does not separate them: a
  // distribution that has moved is a regression to bisect, a tight one behind a
  // single straggler is a runner that was busy. Whoever reads the breach reads
  // this log and nothing else, so the log has to carry the difference.
  console.log(
    `perfbench [fast-3g/390px]: record_open_perceived ` +
      `p50=${nearestRank(samples, 0.5)}ms p99=${nearestRank(samples, 0.99)}ms ` +
      `min=${Math.min(...samples)}ms max=${Math.max(...samples)}ms ` +
      `samples=${JSON.stringify([...samples].sort((a, b) => a - b))}`,
  );
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
