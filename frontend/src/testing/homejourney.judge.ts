// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import {
  receiptGaps,
  worklistGaps,
} from "../features/brief/brief.completeness";
import { greetingNameOf } from "../format/greetingname";

// The Home benchmark's verdicts (e2e/bench-home.spec.ts), kept apart from the
// browser so each one is a plain function a unit test can ask. The marks say
// when the screen thought it was done; these say whether it was.

type Worklist = components["schemas"]["Worklist"];
type MagicReceipt = components["schemas"]["MagicReceipt"];

/** The reads Home's sections draw from. `me` is judged too: whose page it is. */
export const HOME_READS = ["worklist", "handled", "brief", "magic"] as const;
export type HomeRead = (typeof HOME_READS)[number] | "me";

export type Answer = {
  read: HomeRead;
  journey: string;
  status: number;
  body: unknown;
};

/**
 * What the reader's data holds, from `BENCH_HOME_EXPECT`. `work`: the day has
 * rows and a lead. `brief`: the morning brief has items. `handled`: receipts
 * exist. `none`: the reader's day is legitimately empty.
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
  if (words.has("none") && words.size > 1) {
    throw new Error("BENCH_HOME_EXPECT cannot name none beside anything else");
  }
  return {
    work: words.has("work"),
    brief: words.has("brief"),
    handled: words.has("handled"),
  };
}

/** The Home read a request makes, or undefined for any other request. */
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function field(body: unknown, key: string): unknown {
  return isRecord(body) ? body[key] : undefined;
}

function list(body: unknown, key: string): unknown[] {
  const value = field(body, key);
  return Array.isArray(value) ? value : [];
}

function isSourceList(value: unknown): boolean {
  return (
    Array.isArray(value) &&
    value.every(
      (entry) =>
        typeof field(entry, "source") === "string" &&
        typeof field(entry, "reason") === "string",
    )
  );
}

// Checks the fields Home and worklistGaps read. A gap there makes the answer
// partial for this page; the rest of the contract is the api's own tests'.
function isWorklist(body: unknown): body is Worklist {
  const focus = field(body, "focus");
  return (
    isRecord(body) &&
    typeof body.scope === "string" &&
    Array.isArray(body.queue) &&
    Array.isArray(body.counts) &&
    isRecord(body.summary) &&
    isRecord(body.readings) &&
    isSourceList(body.sources_unavailable) &&
    (focus === undefined || Array.isArray(field(focus, "items")))
  );
}

function isReceipt(body: unknown): body is MagicReceipt {
  return isRecord(body) && isSourceList(body.sources_unavailable);
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

/** Sources a grant keeps from this reader: expected, and never degraded. */
export function withheldSources(answers: readonly Answer[]): string[] {
  return answers.flatMap((answer) =>
    list(answer.body, "sources_unavailable")
      .filter((entry) => field(entry, "reason") === "withheld")
      .map((entry) => `${answer.read}:${String(field(entry, "source"))}`),
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
  switch (read) {
    case "worklist":
      return dayReasons(body, expected);
    case "magic":
      return isReceipt(body)
        ? receiptGaps(body)
        : ["magic: answered a partial receipt"];
    case "brief":
      return expected.brief && list(body, "items").length === 0
        ? ["brief: no items for a reader whose brief has some"]
        : [];
    case "handled":
      return expected.handled && list(body, "receipts").length === 0
        ? ["handled: no receipts for a reader who has some"]
        : [];
    case "me":
      return field(field(body, "user"), "id") === reader
        ? []
        : ["me: answered for another reader"];
  }
}

function dayReasons(body: unknown, expected: Expectation): string[] {
  if (!isWorklist(body)) return ["worklist: answered a partial day"];
  const reasons = worklistGaps(body);
  if (expected.work && body.queue.length === 0)
    reasons.push("worklist: empty queue for a reader who has work");
  if (expected.work && body.focus?.items.length === 0)
    reasons.push("worklist: no focus item for a reader who has work");
  return reasons;
}

/** One animation frame: whether the day was on screen, greeting its reader. */
export type Frame = { at: number; day: boolean; reader: boolean };

const lostFrame = (frame: Frame) => !frame.day || !frame.reader;

/** The longest stretch, in ms, of frames that lost the day or its reader. */
export function longestLoss(frames: readonly Frame[]): number {
  let longest = 0;
  let lostSince: number | undefined;
  for (const frame of frames) {
    if (lostFrame(frame)) lostSince ??= frame.at;
    else lostSince = undefined;
    if (lostSince !== undefined)
      longest = Math.max(longest, frame.at - lostSince);
  }
  return Math.round(longest);
}

/**
 * Frames that lost the day or its reader after it first came back; every
 * frame when it never came back at all.
 */
export function framesLostAfterReturn(frames: readonly Frame[]): Frame[] {
  const back = frames.findIndex((frame) => !lostFrame(frame));
  if (back < 0) return [...frames];
  return frames.slice(back).filter(lostFrame);
}

export type Outcome =
  | "complete"
  | "degraded"
  | "useful"
  | "failed"
  | "timed out";

export type Journey = {
  outcome: Outcome;
  times: Record<string, number>;
  reasons: string[];
  withheld: string[];
};

/**
 * A journey's outcome from what the page marked and what it was sent. A page
 * that marked itself complete is still degraded when its answers have gaps.
 */
export function outcomeOf(
  marks: { useful: boolean; failed: boolean },
  settled: boolean,
  reasons: readonly string[],
): Outcome {
  if (marks.failed) return "failed";
  if (!settled) return marks.useful ? "useful" : "timed out";
  return reasons.length > 0 ? "degraded" : "complete";
}

/**
 * The samples of `metric` a budget is judged on, one per journey. A journey
 * that was not complete or never reached it counts as infinite, so a degraded
 * journey can only push a quantile up.
 */
export function budgetSamples(
  journeys: readonly Journey[],
  metric: string,
): number[] {
  return journeys.map((journey) =>
    journey.outcome === "complete"
      ? (journey.times[metric] ?? Number.POSITIVE_INFINITY)
      : Number.POSITIVE_INFINITY,
  );
}

/** How many journeys ended each way, and how often each reason was given. */
export function tally(journeys: readonly Journey[]): {
  outcomes: Map<Outcome, number>;
  reasons: Map<string, number>;
} {
  const outcomes = new Map<Outcome, number>();
  const reasons = new Map<string, number>();
  for (const journey of journeys) {
    outcomes.set(journey.outcome, (outcomes.get(journey.outcome) ?? 0) + 1);
    for (const reason of journey.reasons)
      reasons.set(reason, (reasons.get(reason) ?? 0) + 1);
  }
  return { outcomes, reasons };
}

/** The query client's retries after a server error (app/queryclient.ts). */
export const QUERY_RETRIES = 2;

/**
 * Which journey each Home request belongs to. A request belongs to the
 * journey open when it started, and a retry to the journey of the request it
 * repeats. A retry that lands after the next journey opened is never charged
 * to it. A journey is busy while a request of it is on the wire or a
 * retry of it is still due.
 */
export class RequestLedger {
  private current = "";
  private onWire = new Map<string, number>();
  private retries = new Map<HomeRead, { journey: string; attempts: number }>();

  begin(journey: string): void {
    this.current = journey;
  }

  /** Records a request for `read` and returns the journey it belongs to. */
  started(read: HomeRead): string {
    const due = this.retries.get(read);
    if (due) due.attempts++;
    const journey = due?.journey ?? this.current;
    this.onWire.set(journey, (this.onWire.get(journey) ?? 0) + 1);
    return journey;
  }

  /** Records its end: `status` is null when no response came at all. */
  finished(read: HomeRead, journey: string, status: number | null): void {
    this.onWire.set(journey, (this.onWire.get(journey) ?? 1) - 1);
    // The client retries a server error and nothing else (FE-PARAM-2).
    if (status === null || status < 500) {
      this.retries.delete(read);
      return;
    }
    const due = this.retries.get(read) ?? { journey, attempts: 1 };
    if (due.attempts > QUERY_RETRIES) this.retries.delete(read);
    else this.retries.set(read, due);
  }

  busy(journey: string): boolean {
    if ((this.onWire.get(journey) ?? 0) > 0) return true;
    return [...this.retries.values()].some((due) => due.journey === journey);
  }

  /**
   * Gives up on retries of `journey` that never came. The client does not
   * retry every server error (a 501 is final), so a wait for one is bounded.
   */
  release(journey: string): void {
    for (const [read, due] of this.retries) {
      if (due.journey === journey) this.retries.delete(read);
    }
  }
}
