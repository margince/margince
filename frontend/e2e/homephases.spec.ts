import { createServer, type Server } from "node:http";
import { expect, type Page, test } from "@playwright/test";
import {
  AnswerLog,
  fixedProfile,
  holdWorklist,
  journeyMarks,
  startJourney,
} from "./homejourney";
import { HomePhaseLog } from "./homephases";
import { refocusStaleHome } from "./homevisibility";

let server: Server;
let origin: string;
const DATASET = "home-phases-v1";
const originalCorpus = process.env.BENCH_HOME_CORPUS;
const holds = new Map<string, { entered(): void; answer?: () => void }>();

function holdResponse(id: string) {
  let entered = () => {};
  const ready = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const hold: { entered(): void; answer?: () => void } = { entered };
  holds.set(id, hold);
  return {
    ready,
    release() {
      if (!hold.answer) throw new Error("controlled response has not arrived");
      hold.answer();
      holds.delete(id);
    },
  };
}

test.beforeAll(async () => {
  process.env.BENCH_HOME_CORPUS = DATASET;
  server = createServer((request, response) => {
    const url = new URL(request.url ?? "/", "http://fixture.test");
    if (!url.pathname.startsWith("/v1/")) {
      if (url.pathname.endsWith(".js")) {
        response.setHeader("Content-Type", "application/javascript");
        response.end(
          url.pathname.includes("lazy")
            ? "export const ready = true"
            : "import('./phase-fixture-lazy-v1.js').then(() => document.documentElement.dataset.ready = 'true')",
        );
        return;
      }
      if (url.pathname.endsWith(".css")) {
        response.setHeader("Content-Type", "text/css");
        response.end("main { display: block }");
        return;
      }
      response.setHeader("Content-Type", "text/html");
      response.end(
        "<!doctype html><title>Home timing fixture</title><link rel='stylesheet' href='/assets/phase-fixture-v1.css'><script type='module' src='/assets/phase-fixture-v1.js'></script><main>Retained day</main>",
      );
      return;
    }
    const answer = () => {
      response.statusCode = Number(url.searchParams.get("status") ?? 200);
      response.setHeader("Content-Type", "application/json");
      response.setHeader("Server-Timing", "fixture;dur=7");
      response.setHeader(
        "X-Correlation-ID",
        "11111111-1111-4111-8111-111111111111",
      );
      response.setHeader("Cache-Control", "no-store");
      response.end(JSON.stringify({ fixture: DATASET }));
    };
    const held = holds.get(url.searchParams.get("hold") ?? "");
    if (held) {
      held.answer = answer;
      held.entered();
      return;
    }
    const delay = Number(url.searchParams.get("delay") ?? 0);
    if (delay > 0) setTimeout(answer, delay);
    else answer();
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string")
    throw new Error("fixture has no TCP port");
  origin = `http://127.0.0.1:${address.port}`;
});

test.afterAll(async () => {
  if (originalCorpus === undefined) delete process.env.BENCH_HOME_CORPUS;
  else process.env.BENCH_HOME_CORPUS = originalCorpus;
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
});

async function fixture(page: Page) {
  const phases = await HomePhaseLog.install(page);
  const answers = new AnswerLog(page, phases);
  answers.begin("reload");
  await page.goto(origin);
  await page.waitForFunction(
    () => document.documentElement.dataset.ready === "true",
  );
  return { phases, answers };
}

async function read(
  page: Page,
  path: string,
  renderDelay = 0,
  beforeFetchMs = 0,
) {
  const finished = page.waitForEvent("requestfinished", (request) =>
    request.url().includes(path),
  );
  await page.evaluate(
    async ({ path, renderDelay, beforeFetchMs }) => {
      if (beforeFetchMs > 0)
        await new Promise<void>((resolve) =>
          setTimeout(resolve, beforeFetchMs),
        );
      await fetch(path).then((response) => response.json());
      if (renderDelay > 0)
        await new Promise<void>((resolve) => setTimeout(resolve, renderDelay));
      performance.mark("margince:home:useful");
      performance.mark("margince:home:complete");
    },
    { path, renderDelay, beforeFetchMs },
  );
  await finished;
}

async function snapshot(
  page: Page,
  phases: HomePhaseLog,
  journey = "reload",
  failed = false,
) {
  return phases.snapshot(journey, {
    marks: await journeyMarks(page, "", ["home:useful", "home:complete"]),
    anchorMs: 0,
    kind: "load",
    outcome: failed ? "failed" : "complete",
    reasons: failed ? ["controlled failure"] : [],
  });
}

test("before-fetch delay stays outside browser request total", async ({
  page,
}) => {
  const { phases } = await fixture(page);
  await read(page, "/v1/worklist?scope=mine&filter=all", 0, 200);
  const measured = await snapshot(page, phases);
  expect(measured.census.requests).toBe(1);
  expect(measured.requests[0].phases.beforeFetchMs).toBeGreaterThanOrEqual(190);
  expect(measured.requests[0].phases.totalMs).not.toBeNull();
  await phases.close();
});

test("delayed response is waiting, not time before fetch", async ({ page }) => {
  const { phases } = await fixture(page);
  await read(page, "/v1/worklist?scope=mine&filter=all&delay=200");
  const measured = await snapshot(page, phases);
  expect(measured.requests[0].phases.waitingMs).toBeGreaterThanOrEqual(190);
  expect(measured.requests[0].phases.beforeFetchMs).not.toBeNull();
  expect(measured.requests[0].serverTiming).toEqual([
    { name: "fixture", durationMs: 7 },
  ]);
  expect(measured.requests[0].correlation).toBe(
    "11111111-1111-4111-8111-111111111111",
  );
  await phases.close();
});

test("rendering after response has its own readiness interval", async ({
  page,
}) => {
  const { phases } = await fixture(page);
  await read(page, "/v1/worklist?scope=mine&filter=all", 200);
  const measured = await snapshot(page, phases);
  expect(measured.readiness.afterResponseMs).toBeGreaterThanOrEqual(190);
  expect(measured.readiness.completeSinceDocumentMs).not.toBeNull();
  await phases.close();
});

test("reload and route return retain separate comparable request censuses", async ({
  page,
}) => {
  const { phases, answers } = await fixture(page);
  await read(page, "/v1/worklist?scope=mine&filter=all");
  const loaded = await snapshot(page, phases);
  answers.begin("return-1");
  await startJourney(page, "return-1");
  await Promise.all([
    read(page, "/v1/worklist?scope=mine&filter=all"),
    read(page, "/v1/brief"),
  ]);
  const returned = await phases.snapshot("return-1", {
    marks: {},
    anchorMs: 0,
    kind: "return",
    outcome: "useful",
    reasons: [],
  });
  expect(loaded.census.requests).toBe(1);
  expect(returned.census.requests).toBe(2);
  expect(returned.requests.map((item) => item.read).sort()).toEqual([
    "brief",
    "worklist",
  ]);
  expect(returned.metadata.bundles).toEqual(loaded.metadata.bundles);
  expect(loaded.metadata.bundles).toContain("/assets/phase-fixture-lazy-v1.js");
  expect(loaded.metadata.bundles).toContain("/assets/phase-fixture-v1.css");
  expect(returned.metadata.declaredCorpus).toBe(loaded.metadata.declaredCorpus);
  expect(loaded.metadata.declaredCorpus).toBe(DATASET);
  expect(returned.readiness.completeSinceDocumentMs).toBeNull();
  await page.reload();
  answers.begin("reload-2");
  await read(page, "/v1/worklist?scope=mine&filter=all");
  const reloaded = await snapshot(page, phases, "reload-2");
  expect(reloaded.census.requests).toBe(1);
  expect(reloaded.metadata.navigation).toEqual(["reload"]);
  await phases.close();
});

test("a server failure and its retry keep the original journey", async ({
  page,
}) => {
  const { phases, answers } = await fixture(page);
  await read(page, "/v1/worklist?scope=mine&filter=all&status=503");
  answers.begin("return-later");
  await read(page, "/v1/worklist?scope=mine&filter=all");
  const measured = await snapshot(page, phases, "reload", true);
  expect(measured.requests.map((item) => item.status)).toEqual([503, 200]);
  expect(measured.requests.map((item) => item.attempt)).toEqual([1, 2]);
  expect(measured.census.retries).toBe(1);
  expect(measured.readiness.completeSinceDocumentMs).toBeNull();
  expect((await snapshot(page, phases, "return-later")).census.requests).toBe(
    0,
  );
  await phases.close();
});

test("cancelled requests keep unavailable timings and cannot be fast success", async ({
  page,
}) => {
  const { phases } = await fixture(page);
  const requested = page.waitForEvent("request", (request) =>
    request.url().includes("/v1/worklist"),
  );
  const failed = page.waitForEvent("requestfailed");
  const pending = page.evaluate(async () => {
    const controller = new AbortController();
    window.addEventListener("fixture-abort", () => controller.abort(), {
      once: true,
    });
    try {
      await fetch("/v1/worklist?scope=mine&filter=all&delay=200", {
        signal: controller.signal,
      });
    } catch (error) {
      if (!(error instanceof DOMException) || error.name !== "AbortError")
        throw error;
    }
  });
  await requested;
  await page.evaluate(() => window.dispatchEvent(new Event("fixture-abort")));
  await Promise.all([pending, failed]);
  const measured = await snapshot(page, phases, "reload", true);
  expect(measured.census.failed).toBe(1);
  expect(measured.census.cancelled).toBe(1);
  expect(measured.requests[0].phases.waitingMs).toBeNull();
  expect(measured.readiness.completeSinceDocumentMs).toBeNull();
  await phases.close();
});

test("cancellation before browser dispatch remains visible in the fetch census", async ({
  page,
}) => {
  const { phases } = await fixture(page);
  await page.evaluate(async () => {
    const controller = new AbortController();
    controller.abort();
    try {
      await fetch("/v1/worklist?scope=mine&filter=all", {
        signal: controller.signal,
      });
    } catch (error) {
      if (!(error instanceof DOMException) || error.name !== "AbortError")
        throw error;
    }
  });
  const measured = await snapshot(page, phases, "reload", true);
  expect(measured.census.requests).toBe(0);
  expect(measured.census.unmatchedFetchCallsInDocument).toBe(1);
  expect(measured.census.fetchCancelledInDocument).toBe(1);
  expect(measured.readiness.completeSinceDocumentMs).toBeNull();
  await phases.close();
});

test("an unfinished response stays in the census without cache evidence", async ({
  page,
}) => {
  const { phases } = await fixture(page);
  const held = holdResponse("unfinished");
  const pending = read(
    page,
    "/v1/worklist?scope=mine&filter=all&hold=unfinished",
  );
  await held.ready;
  try {
    const measured = await snapshot(page, phases);
    expect(measured.census.requests).toBe(1);
    expect(measured.census.inFlight).toBe(1);
    expect(measured.incomplete).toBe(true);
    expect(measured.judgedComplete).toBe(false);
    expect(measured.requests[0].cached).toBeNull();
    await expect(page.locator("main")).toHaveText("Retained day");
  } finally {
    held.release();
    await pending;
  }
  expect((await snapshot(page, phases)).census.inFlight).toBe(0);
  await phases.close();
});

test("navigation keeps an unfinished old-document request in its original journey", async ({
  page,
}) => {
  const { phases } = await fixture(page);
  const held = holdResponse("navigation");
  const pending = page
    .evaluate(() => fetch("/v1/worklist?scope=mine&filter=all&hold=navigation"))
    .then(
      () => null,
      (error: unknown) =>
        error instanceof Error ? error.message : String(error),
    );
  await held.ready;
  try {
    await page.goto(`${origin}/?next-document`);
    expect(await pending).toMatch(/Execution context was destroyed|navigation/);
    const measured = await snapshot(page, phases, "reload", true);
    expect(measured.census.requests).toBe(1);
    expect(measured.census.inFlight + measured.census.failed).toBe(1);
    expect(measured.census.abandonedDocuments).toBe(1);
    expect(measured.incomplete).toBe(measured.census.inFlight > 0);
    expect(measured.requests[0].startSinceDocumentMs).toBeNull();
    expect(measured.judgedComplete).toBe(false);
  } finally {
    held.release();
  }
  await phases.close();
});

test("same-URL reads retain send evidence beyond the browser connection limit", async ({
  page,
}) => {
  const { phases } = await fixture(page);
  await fixedProfile(page);
  const completed = new Promise<void>((resolve) => {
    let count = 0;
    page.on("requestfinished", (request) => {
      if (request.url().includes("/v1/worklist") && ++count === 10) resolve();
    });
  });
  await page.evaluate(() =>
    Promise.all(
      Array.from({ length: 10 }, () =>
        fetch("/v1/worklist?scope=mine&filter=all&delay=100").then((response) =>
          response.json(),
        ),
      ),
    ),
  );
  await completed;
  const measured = await snapshot(page, phases);
  expect(measured.census.requests).toBe(10);
  expect(
    measured.requests.every((request) => request.phases.sendMs !== null),
  ).toBe(true);
  expect(
    measured.requests.every((request) => request.phases.waitingMs !== null),
  ).toBe(true);
  expect(
    measured.requests.every((request) => request.phases.connectMs !== null),
  ).toBe(true);
  expect(measured.metadata.bundles).toContain(
    "/assets/phase-fixture-lazy-v1.js",
  );
  await phases.close();
});

test("the actual refresh hold cannot be reported as API network waiting", async ({
  page,
}) => {
  const { phases } = await fixture(page);
  const held = await holdWorklist(page, phases.intercept);
  const requested = page.waitForEvent("request", (request) =>
    request.url().includes("/v1/worklist"),
  );
  const pending = read(page, "/v1/worklist?scope=mine&filter=all");
  await requested;
  await held.ready;
  await held.release();
  await pending;
  const measured = await snapshot(page, phases);
  expect(measured.requests[0].intercepted).toBe(true);
  expect(measured.requests[0].phases.waitingMs).toBeNull();
  expect(measured.requests[0].phases.sendMs).toBeNull();
  expect(measured.requests[0].phases.totalMs).toBeNull();
  expect(measured.requests[0].harnessElapsedMs).not.toBeNull();
  await phases.close();
});

test("refocus ages Date without moving performance time or leaving visibility mocked", async ({
  page,
}) => {
  const { phases } = await fixture(page);
  const before = await page.evaluate(() => ({
    date: Date.now(),
    mono: performance.now(),
    origin: performance.timeOrigin,
  }));
  await page.evaluate(() =>
    document.addEventListener("visibilitychange", () =>
      performance.mark("fixture:visibility", {
        detail: { state: document.visibilityState, hidden: document.hidden },
      }),
    ),
  );
  const advance = await refocusStaleHome(page);
  const after = await page.evaluate(() => ({
    date: Date.now(),
    mono: performance.now(),
    origin: performance.timeOrigin,
    hidden: document.hidden,
    state: document.visibilityState,
    events: performance
      .getEntriesByName("fixture:visibility", "mark")
      .flatMap((entry) => {
        if (!(entry instanceof PerformanceMark)) return [];
        const detail: unknown = entry.detail;
        return [detail];
      }),
  }));
  expect(after.date - before.date).toBeGreaterThanOrEqual(advance);
  expect(after.mono).toBeGreaterThanOrEqual(before.mono);
  expect(after.mono - before.mono).toBeLessThan(advance / 2);
  expect(after.origin).toBe(before.origin);
  expect(after.events).toEqual([
    { state: "hidden", hidden: true },
    { state: "visible", hidden: false },
  ]);
  expect(after.state).toBe("visible");
  expect(after.hidden).toBe(false);
  await phases.close();
});
