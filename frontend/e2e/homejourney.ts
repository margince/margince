import type { Page, Request, Response, Route } from "@playwright/test";
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
