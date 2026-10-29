import type { Page, Request, Response } from "@playwright/test";
import { greetingNameOf } from "../src/format/greetingname";
import {
  JOURNEY_START,
  type JourneyMark,
  journeyMarkName,
  journeyOf,
} from "../src/lib/journeymarks";

/**
 * What the Home benchmark checks besides time: the answers the page itself
 * received, judged against what the signed-in reader is known to have. The
 * marks say when the screen thought it was done; these say whether it was.
 */

/** The reads Home's sections draw from. `me` is judged too: whose page it is. */
export const HOME_READS = ["worklist", "handled", "brief", "magic"] as const;
export type HomeRead = (typeof HOME_READS)[number] | "me";

export type Answer = { read: HomeRead; status: number; body: unknown };

/**
 * What the reader's data holds, from `BENCH_HOME_EXPECT`: `work` (the day has
 * rows), `brief` (the morning brief has items), `handled` (receipts exist), or
 * `none` for a reader whose day is legitimately empty.
 */
export type Expectation = { work: boolean; brief: boolean; handled: boolean };

export function expectationFrom(spelled: string): Expectation {
  const words = new Set(spelled.split(",").map((word) => word.trim()));
  for (const word of words) {
    if (!["work", "brief", "handled", "none"].includes(word)) {
      throw new Error(
        `BENCH_HOME_EXPECT names "${word}"; use work, brief, handled or none`,
      );
    }
  }
  return {
    work: words.has("work"),
    brief: words.has("brief"),
    handled: words.has("handled"),
  };
}

/** The Home read a response answers, or undefined for any other request. */
export function readOf(url: URL, method: string): HomeRead | undefined {
  if (method !== "GET" || url.searchParams.has("cursor")) return undefined;
  switch (url.pathname) {
    case "/v1/worklist":
      return url.searchParams.get("scope") === "mine" &&
        url.searchParams.get("filter") === "all" &&
        !url.searchParams.has("owner")
        ? "worklist"
        : undefined;
    case "/v1/worklist/handled":
      return "handled";
    case "/v1/brief":
      return "brief";
    case "/v1/magic":
      return "magic";
    case "/v1/me":
      return "me";
    default:
      return undefined;
  }
}

/** The signed-in reader `/v1/me` names, and the name Home greets them by. */
export function readerFrom(body: unknown): {
  id: string;
  greeting: string | null;
} {
  const user = field(body, "user");
  const id = field(user, "id");
  if (typeof id !== "string") throw new Error("/v1/me named no user id");
  const text = (key: string) => {
    const value = field(user, key);
    return typeof value === "string" ? value : null;
  };
  return {
    id,
    greeting: greetingNameOf({
      greeting_name: text("greeting_name"),
      display_name: text("display_name"),
    }),
  };
}

function field(body: unknown, key: string): unknown {
  return typeof body === "object" && body !== null && key in body
    ? Object.entries(body).find(([name]) => name === key)?.[1]
    : undefined;
}

function list(body: unknown, key: string): unknown[] {
  const value = field(body, key);
  return Array.isArray(value) ? value : [];
}

function sourcesWith(body: unknown, reason: string): string[] {
  return list(body, "sources_unavailable")
    .filter((entry) => field(entry, "reason") === reason)
    .map((entry) => String(field(entry, "source")));
}

/** Sources a grant keeps from this reader: expected, and never degraded. */
export function withheldSources(answers: readonly Answer[]): string[] {
  return answers.flatMap((answer) =>
    sourcesWith(answer.body, "withheld").map((s) => `${answer.read}:${s}`),
  );
}

/**
 * Why the answers do not make a healthy Home for `reader`, or [] when they do.
 * Every answer counts, a retry included: a later good answer does not erase a
 * failed one, because the reader saw the page in between.
 */
export function degradedReasons(
  answers: readonly Answer[],
  expected: Expectation,
  reader: string,
): string[] {
  const reasons: string[] = [];
  for (const read of HOME_READS) {
    if (!answers.some((answer) => answer.read === read)) {
      reasons.push(`${read}: no answer`);
    }
  }
  for (const answer of answers) {
    reasons.push(...answerReasons(answer, expected, reader));
  }
  return reasons;
}

function answerReasons(
  { read, status, body }: Answer,
  expected: Expectation,
  reader: string,
): string[] {
  // No brief run yet is a page that offers to make one, unless one is expected.
  if (read === "brief" && status === 404 && !expected.brief) return [];
  if (status !== 200) return [`${read}: answered ${status}`];
  if (body === undefined) return [`${read}: answered 200 with no JSON body`];
  const reasons = sourcesWith(body, "failed").map(
    (source) => `${read}: source ${source} failed`,
  );
  if (read === "worklist" && expected.work && list(body, "queue").length === 0)
    reasons.push("worklist: empty queue for a reader who has work");
  if (read === "brief" && expected.brief && list(body, "items").length === 0)
    reasons.push("brief: no items for a reader whose brief has some");
  if (
    read === "handled" &&
    expected.handled &&
    list(body, "receipts").length === 0
  )
    reasons.push("handled: no receipts for a reader who has some");
  if (read === "me" && field(field(body, "user"), "id") !== reader)
    reasons.push("me: answered for another reader");
  return reasons;
}

/**
 * Every Home answer the page receives, in batches: `take` hands over what
 * arrived since the last take, once each body has been read.
 */
export class AnswerLog {
  private pending: Promise<Answer>[] = [];
  private open = 0;

  constructor(page: Page) {
    const isHomeRead = (request: Request) =>
      readOf(new URL(request.url()), request.method()) !== undefined;
    page.on("request", (request) => {
      if (isHomeRead(request)) this.open++;
    });
    const settle = (request: Request) => {
      if (isHomeRead(request)) this.open--;
    };
    page.on("requestfinished", settle);
    page.on("requestfailed", settle);
    page.on("response", (response) => {
      const read = readOf(new URL(response.url()), response.request().method());
      if (read) this.pending.push(answerOf(read, response));
    });
  }

  /**
   * Home reads still on the wire. A refetch asked for while one is in flight
   * joins it and sends nothing, so a journey starts only once this is 0.
   */
  inFlight(): number {
    return this.open;
  }

  /** Whether every read in `reads` has answered since the last take. */
  async holds(reads: readonly HomeRead[]): Promise<boolean> {
    const answers = await Promise.all(this.pending);
    return reads.every((read) => answers.some((a) => a.read === read));
  }

  async take(): Promise<Answer[]> {
    const taken = this.pending;
    this.pending = [];
    return Promise.all(taken);
  }
}

async function answerOf(read: HomeRead, response: Response): Promise<Answer> {
  const status = response.status();
  try {
    return { read, status, body: await response.json() };
  } catch {
    // Judged in answerReasons: a 200 without a body is named, never read as an
    // empty day.
    return { read, status, body: undefined };
  }
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

const GLANCE = '[data-testid="brief-glance"]';

/** One animation frame: whether the day was on screen, greeting `reader`. */
export type Frame = { at: number; day: boolean; reader: boolean };

/**
 * Samples the screen every frame until `stop`, so a moment the day left the
 * screen or showed someone else is seen even if it lasted one frame.
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
 * Times the next click on `target` from the browser's own input timestamp,
 * which includes any wait for a busy main thread, to the first frame in which
 * `answer` matches.
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
