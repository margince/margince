import type { Page, Request, Response, Route } from "@playwright/test";
import { HOME_DAY_TIMING } from "../src/lib/elementtiming";
import {
  JOURNEY_START,
  type JourneyMark,
  journeyMarkName,
  journeyOf,
} from "../src/lib/journeymarks";
import {
  type Answer,
  type Frame,
  type HomeRead,
  RequestLedger,
  readOf,
  type Vitals,
} from "../src/testing/homejourney.judge";

/**
 * The browser side of the Home benchmark: what the page was sent, what its
 * screen showed and when. The verdicts on all of it are plain functions in
 * src/testing/homejourney.judge.ts.
 */

const homeRead = (request: Request) =>
  readOf(new URL(request.url()), request.method());

/**
 * Every Home answer the page receives, filed under the journey its request
 * belongs to (RequestLedger). A late answer or a retry is never judged as
 * part of the next journey.
 */
export class AnswerLog {
  private readonly ledger = new RequestLedger();
  private readonly owners = new WeakMap<Request, string>();
  private readonly statuses = new WeakMap<Request, number>();
  private answers: { journey: string; answer: Promise<Answer> }[] = [];

  constructor(page: Page) {
    page.on("request", (request) => {
      const read = homeRead(request);
      if (read) this.owners.set(request, this.ledger.started(read));
    });
    page.on("response", (response) => {
      const request = response.request();
      const read = homeRead(request);
      const journey = this.owners.get(request);
      if (!read || journey === undefined) return;
      this.statuses.set(request, response.status());
      this.answers.push({ journey, answer: answerOf(read, journey, response) });
    });
    const finish = (request: Request) => {
      const read = homeRead(request);
      const journey = this.owners.get(request);
      if (read && journey !== undefined)
        this.ledger.finished(read, journey, this.statuses.get(request) ?? null);
    };
    page.on("requestfinished", finish);
    page.on("requestfailed", finish);
  }

  /** Files every Home request from now on under `journey`. */
  begin(journey: string): void {
    this.ledger.begin(journey);
  }

  /** Whether `journey` is done: every read in `reads` answered, none due. */
  async settled(journey: string, reads: readonly HomeRead[]): Promise<boolean> {
    if (this.ledger.busy(journey)) return false;
    const answers = await this.of(journey);
    return reads.every((read) => answers.some((a) => a.read === read));
  }

  /** The answers of `journey`, and the end of waiting for any more. */
  async close(journey: string): Promise<Answer[]> {
    this.ledger.release(journey);
    const answers = await this.of(journey);
    this.answers = this.answers.filter((a) => a.journey !== journey);
    return answers;
  }

  private of(journey: string): Promise<Answer[]> {
    return Promise.all(
      this.answers.filter((a) => a.journey === journey).map((a) => a.answer),
    );
  }
}

async function answerOf(
  read: HomeRead,
  journey: string,
  response: Response,
): Promise<Answer> {
  const status = response.status();
  try {
    return { read, journey, status, body: await response.json() };
  } catch {
    // Judged in degradedReasons: a 200 without a body is named, never read as
    // an empty day.
    return { read, journey, status, body: undefined };
  }
}

/**
 * Holds every answer to the day's read until `release`, so a click can be
 * made while the refetch is still on the wire. The server does its work at
 * once; only the answer waits.
 */
export async function holdWorklist(page: Page) {
  let open = () => {};
  const opened = new Promise<void>((resolve) => {
    open = resolve;
  });
  const answering: Promise<void>[] = [];
  const matches = (url: URL) => readOf(url, "GET") === "worklist";
  const hold = (route: Route) => {
    const answer = (async () => {
      if (route.request().method() !== "GET") return route.continue();
      const response = await route.fetch();
      await opened;
      return route.fulfill({ response });
    })();
    answering.push(answer);
    return answer;
  };
  await page.route(matches, hold);
  return {
    // Unrouting first would hand the held answers to the default handler.
    release: async () => {
      open();
      await Promise.all(answering);
      await page.unroute(matches, hold);
    },
  };
}

/** Opens a journey in the running document; returns when it opened. */
export function startJourney(page: Page, id: string): Promise<number> {
  return page.evaluate(
    ({ name, detail }) => performance.mark(name, { detail }).startTime,
    { name: JOURNEY_START, detail: id },
  );
}

/** The time of each mark set for journey `id` ("" for the document load). */
export async function journeyMarks(
  page: Page,
  id: string,
  marks: readonly JourneyMark[],
): Promise<Partial<Record<JourneyMark, number>>> {
  const entries = await page.evaluate(() =>
    performance.getEntriesByType("mark").map((entry) => ({
      name: entry.name,
      startTime: entry.startTime,
      detail: "detail" in entry ? entry.detail : null,
    })),
  );
  const out: Partial<Record<JourneyMark, number>> = {};
  for (const mark of marks) {
    const entry = entries.find(
      (e) => e.name === journeyMarkName(mark) && journeyOf(e) === id,
    );
    if (entry) out[mark] = entry.startTime;
  }
  return out;
}

export const GLANCE = '[data-testid="brief-glance"]';

/**
 * Samples the screen every frame until `stop`. A moment the day left the
 * screen or showed someone else is seen, even if it lasted one frame.
 */
export function watchScreen(page: Page, day: string, greeting: string | null) {
  return page.evaluateHandle(
    ({ day, glance, greeting }) => {
      const frames: Frame[] = [];
      let running = true;
      const tick = (at: number) => {
        if (!running) return;
        const text = document.querySelector(glance)?.textContent ?? "";
        frames.push({
          at,
          day: document.querySelector(day) !== null,
          reader: greeting === null || text.includes(greeting),
        });
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
      return {
        stop: () => {
          running = false;
          return frames;
        },
      };
    },
    { day, glance: GLANCE, greeting },
  );
}

/**
 * Times the next click on `target` from the browser's own input timestamp to
 * the first frame in which `answer` matches. The input timestamp includes any
 * wait for a busy main thread.
 */
export function probeClick(page: Page, target: string, answer: string) {
  return page.evaluateHandle(
    ({ target, answer }) => {
      let clickAt: number | undefined;
      let answeredAt: number | undefined;
      const onClick = (event: Event) => {
        if (event.target instanceof Element && event.target.closest(target))
          clickAt ??= event.timeStamp;
      };
      document.addEventListener("click", onClick, { capture: true });
      const watcher = new MutationObserver(() => {
        if (clickAt === undefined || answeredAt !== undefined) return;
        if (document.querySelector(answer) === null) return;
        requestAnimationFrame((at) => {
          answeredAt ??= at;
        });
      });
      watcher.observe(document.body, {
        subtree: true,
        childList: true,
        attributes: true,
      });
      return {
        result: () => {
          watcher.disconnect();
          document.removeEventListener("click", onClick, { capture: true });
          return { clickAt, answeredAt };
        },
      };
    },
    { target, answer },
  );
}

/** The spans of the day's refetches that started at or after `from`. */
export function worklistFetches(page: Page, from: number) {
  return page.evaluate(
    (from) =>
      performance
        .getEntriesByType("resource")
        .filter(
          (entry) =>
            entry.startTime >= from &&
            new URL(entry.name).pathname === "/v1/worklist",
        )
        .map((entry) => ({
          start: entry.startTime,
          end: entry.startTime + entry.duration,
        })),
    from,
  );
}

const VITALS_KEY = "__homeVitals";

/**
 * Records, from the start of every document the page loads, each Element
 * Timing paint of Home's day content and each layout shift.
 *
 * Chrome times a text paint only on the block that holds the text, never on a
 * marked block around it. So every element inside the marked content takes
 * the mark too, before it first paints.
 */
export async function recordVitals(page: Page): Promise<void> {
  // Chrome times a block that fades in only when its fade ends. Home fades its
  // content in, unless the reader asks for reduced motion.
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.addInitScript(
    ({ key, mark }) => {
      const vitals: Vitals = { dayPaints: [], shifts: [] };
      Object.assign(window, { [key]: vitals });
      const marked = `[elementtiming="${mark}"]`;
      new MutationObserver((records) => {
        for (const record of records) {
          for (const node of record.addedNodes) {
            if (!(node instanceof Element)) continue;
            // React adds a whole tree at once, so the mark may sit below it.
            const roots = node.closest(marked)
              ? [node]
              : node.querySelectorAll(marked);
            for (const root of roots) {
              for (const inner of [root, ...root.querySelectorAll("*")])
                inner.setAttribute("elementtiming", mark);
            }
          }
        }
      }).observe(document, { childList: true, subtree: true });
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) {
          if (!("identifier" in entry) || entry.identifier !== mark) continue;
          const render = "renderTime" in entry ? entry.renderTime : 0;
          const load = "loadTime" in entry ? entry.loadTime : 0;
          const rect =
            "intersectionRect" in entry ? entry.intersectionRect : null;
          if (typeof render !== "number" || typeof load !== "number") continue;
          if (!(rect instanceof DOMRectReadOnly)) continue;
          vitals.dayPaints.push({
            at: render || load,
            area: rect.width * rect.height,
          });
        }
      }).observe({ type: "element", buffered: true });
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) {
          const value = "value" in entry ? entry.value : undefined;
          const input = "hadRecentInput" in entry && entry.hadRecentInput;
          if (typeof value !== "number") continue;
          vitals.shifts.push({
            at: entry.startTime,
            value,
            input: input === true,
          });
        }
      }).observe({ type: "layout-shift", buffered: true });
    },
    { key: VITALS_KEY, mark: HOME_DAY_TIMING },
  );
}

/** What the current document has painted and shifted so far. */
export async function readVitals(page: Page): Promise<Vitals> {
  const vitals: Vitals | null = await page.evaluate(
    (key) => Reflect.get(window, key) ?? null,
    VITALS_KEY,
  );
  if (vitals === null) {
    throw new Error(
      "the page holds no vitals: call recordVitals before it loads",
    );
  }
  return vitals;
}

/**
 * The vitals once the day content's paints are in. The browser reports a paint
 * frames after it, and text waits for its font, so Home's own marks come first.
 */
export async function settledVitals(
  page: Page,
  timeout: number,
): Promise<Vitals> {
  // A load that never paints is the judge's to fail, not an error here.
  await page
    .waitForFunction(
      (key) => {
        const vitals: unknown = Reflect.get(window, key);
        return (
          typeof vitals === "object" &&
          vitals !== null &&
          "dayPaints" in vitals &&
          Array.isArray(vitals.dayPaints) &&
          vitals.dayPaints.length > 0
        );
      },
      VITALS_KEY,
      { timeout },
    )
    .catch(() => undefined);
  await page.evaluate(async () => {
    await document.fonts.ready;
    for (let frame = 0; frame < 2; frame++)
      await new Promise((done) => requestAnimationFrame(done));
  });
  return readVitals(page);
}

/**
 * The fixed profile the vitals budgets hold on: a desktop CPU slowed twofold,
 * and a 40 ms, 10 Mbit/s link. Fixed, so two runs measure the same machine.
 */
export async function fixedProfile(page: Page): Promise<void> {
  const session = await page.context().newCDPSession(page);
  await session.send("Emulation.setCPUThrottlingRate", { rate: 2 });
  await session.send("Network.enable");
  await session.send("Network.emulateNetworkConditions", {
    offline: false,
    latency: 40,
    downloadThroughput: (10 * 1024 * 1024) / 8,
    uploadThroughput: (5 * 1024 * 1024) / 8,
  });
}
