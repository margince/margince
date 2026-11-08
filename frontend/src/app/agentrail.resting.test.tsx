// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider, translatePlural } from "../i18n";
import { en, type MessageKey } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { AgentRail } from "./agentrail";
import {
  restingReadings,
  stillNews,
  useRestingLine,
} from "./agentrail-resting";
import { plain, type SpokenLine, spokenText } from "./ai-activity-speak";

// What the rail says when nothing is happening, which is most of the day.
//
// The whole file is written against the failure it was opened for: the resting
// line said the SAME SENTENCE from the moment a run finished until midnight,
// because the rotation carried the newest settled run and nothing else could
// join it. So the cases below are about MOVEMENT and about STALENESS, and each
// one names which.
//
// The clock is injected everywhere it matters. `stillNews` takes `now` as an
// argument, and the rotation's timers run on vitest's fake clock — so nothing
// here can pass because the machine was quick or fail because it was busy, and
// nothing goes red when the calendar moves (`make fe-clock-drift`).

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;

/** A settled occurrence, dated relative to the case's own "now". */
function run(finishedAgo: number, at = 0) {
  return {
    started_at: new Date(at - finishedAgo - MINUTE).toISOString(),
    finished_at: new Date(at - finishedAgo).toISOString(),
  };
}

/** The words of whichever line is showing. */
function said(line: SpokenLine): string {
  return spokenText(line);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("stillNews", () => {
  it("keeps a run that has just settled and drops one from this morning", () => {
    const now = 0;
    const kept = run(10 * MINUTE, now);
    const stale = run(5 * HOUR, now);
    expect(stillNews([kept, stale], now)).toEqual([kept]);
  });

  // The bound is what stops the rail announcing breakfast after lunch, and it
  // is the whole reason this function exists — so it is asserted at the hour
  // rather than only at the extremes.
  it("keeps a run from two hours ago and drops one from four", () => {
    const now = 0;
    expect(stillNews([run(2 * HOUR, now)], now)).toHaveLength(1);
    expect(stillNews([run(4 * HOUR, now)], now)).toHaveLength(0);
  });

  // The feed serves up to ten and the rotation is not a recap. Asserted on the
  // ORDER too: the feed answers newest first and a rotation that reordered them
  // would announce the older of two runs as the newer.
  it("carries only the newest three, in the order the feed gave them", () => {
    const now = 0;
    const items = [1, 2, 3, 4, 5].map((n) => run(n * MINUTE, now));
    expect(stillNews(items, now)).toEqual(items.slice(0, 3));
  });

  // Under-reporting is the one way this selection must not fail: a settled run
  // the rail silently refuses to mention looks exactly like a quiet day. A row
  // with no finish time is dated by its start, which can only age it out
  // EARLIER than the truth — never keep a stale one, and never drop a fresh one
  // for want of a field.
  it("dates a run with no finish time by when it started", () => {
    const now = 0;
    const fresh = { started_at: new Date(now - MINUTE).toISOString() };
    const old = { started_at: new Date(now - 6 * HOUR).toISOString() };
    expect(stillNews([fresh, old], now)).toEqual([fresh]);
  });

  it("drops a row whose timestamps are not dates rather than keeping it", () => {
    expect(stillNews([{ started_at: "not a date" }], 0)).toEqual([]);
  });
});

describe("restingReadings", () => {
  const QUIET = {
    waiting: undefined,
    sourcesUnknown: false,
    developmentLine: null,
    settled: [] as readonly SpokenLine[],
  };

  // The words the rotation says its readings in, as the rail hands them over:
  // the English catalogue, and the queue's line through the plural rule rather
  // than a number pasted onto a noun.
  const WORDS = {
    t: (key: MessageKey) => en[key],
    waiting: (count: number) =>
      translatePlural("en", "agent.line.waiting", count, {
        count: String(count),
      }),
  };

  // The rule the rotation has always held: a read that has not answered is
  // ABSENT, never a zero standing in for an all-clear.
  it("says nothing needs you when every read came back with nothing", () => {
    expect(restingReadings(QUIET, WORDS).map(said)).toEqual([
      "Nothing needs attention",
    ]);
  });

  it("does not count an unanswered approvals read as a clean queue", () => {
    const answered = restingReadings({ ...QUIET, waiting: 0 }, WORDS);
    expect(answered.map(said)).toEqual(["Nothing needs attention"]);
  });

  it("claims no all-clear when the connections read failed, and keeps saying so beside other news", () => {
    const unknown = { ...QUIET, sourcesUnknown: true };
    expect(restingReadings(unknown, WORDS).map(said)).toEqual([
      "Could not check your connected sources",
    ]);
    // Other news does not push the failed check out of the rotation.
    expect(
      restingReadings({ ...unknown, waiting: 2 }, WORDS).map(said),
    ).toEqual([
      "Could not check your connected sources",
      "2 approvals waiting",
    ]);
  });

  // The fix for the pinned line: a day that settled three runs says three
  // things, in severity order behind the queue a contact has to answer.
  it("carries every fresh run, after what is waiting and before the model", () => {
    const lines = restingReadings(
      {
        waiting: 2,
        sourcesUnknown: false,
        developmentLine: "offline model",
        settled: [plain("brief ready"), plain("summary ready")],
      },
      WORDS,
    );
    expect(lines.map(said)).toEqual([
      en["agent.line.waiting_other"].replace("{count}", "2"),
      "brief ready",
      "summary ready",
      "offline model",
    ]);
  });
});

describe("useRestingLine", () => {
  const READINGS = [plain("one"), plain("two")];

  /** Runs the rotation forward far enough for the next line to take over. */
  function advance(ms: number) {
    act(() => {
      vi.advanceTimersByTime(ms);
    });
  }

  it("walks the readings in turn and starts over", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useRestingLine(READINGS));
    const seen = [said(result.current)];
    for (let step = 0; step < 3; step += 1) {
      advance(6_000);
      seen.push(said(result.current));
    }
    expect(seen).toEqual(["one", "two", "one", "two"]);
  });

  // A rail with one reading holds it rather than flickering against itself.
  it("stands still on a single reading", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useRestingLine([plain("only this")]));
    advance(60_000);
    expect(said(result.current)).toBe("only this");
  });

  // The readings can shorten under the rotation when a read answers, and an index
  // left pointing past the end would blank the line.
  it("keeps saying something when the readings shorten under it", () => {
    vi.useFakeTimers();
    const { rerender, result } = renderHook(
      ({ lines }: { lines: readonly SpokenLine[] }) => useRestingLine(lines),
      { initialProps: { lines: [plain("a"), plain("b"), plain("c")] } },
    );
    advance(6_000);
    advance(6_000);
    expect(said(result.current)).toBe("c");
    rerender({ lines: [plain("a")] });
    expect(said(result.current)).toBe("a");
  });
});

// The same two claims again, through the rendered rail.
//
// The units above prove the SELECTION — which runs are still news, and what a
// resting rotation is made of. They cannot prove the rail asks either question,
// and that is the half that would break silently: a rail that stopped passing
// its feed through `stillNews` leaves every case above green and puts this
// morning's summary back in the corner of every screen until midnight.

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

type AiActivityItem = components["schemas"]["AiActivityItem"];

/** A run that finished `agoMs` ago. */
function settledRun(
  kind: string,
  agoMs: number,
  over: Partial<AiActivityItem> = {},
): AiActivityItem {
  const finished = Date.now() - agoMs;
  return {
    id: `019f7e65-0000-7000-8000-0000000000${kind.length}3`,
    kind: kind as AiActivityItem["kind"],
    state: "done",
    started_at: new Date(finished - MINUTE).toISOString(),
    finished_at: new Date(finished).toISOString(),
    ...over,
  };
}

/** The sentence from the screenshot this change was opened against. */
const SUMMARY_LINE = en["agent.activity.summarizeNamed.done"].replace(
  "{name}",
  "Sabine Mayer",
);
const BRIEF_LINE = "Morning brief ready";

const summary = (agoMs: number) =>
  settledRun("summarize", agoMs, { subject_label: "Sabine Mayer" });

/** Every read the section makes, with the feed answered by the case. */
function stubRail(recent: readonly AiActivityItem[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/me/ai-activity")) {
        return jsonResponse({ running: [], recent, faults: [] });
      }
      if (pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow: {} }));
      }
      if (pathname.endsWith("/assistant/profile")) {
        return jsonResponse({ state: "configured" });
      }
      if (pathname.endsWith("/connectors")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse({
        data: [],
        page: { has_more: false, next_cursor: null },
      });
    }),
  );
}

function mountRail() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <AgentRail />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const shown = (container: HTMLElement) =>
  container.querySelector(".arline")?.textContent;

describe("the rail's resting line", () => {
  /** Every sentence the rotation says over a full pass and a bit more. */
  async function walk(container: HTMLElement, steps: number) {
    const seen = [shown(container)];
    for (let step = 0; step < steps; step += 1) {
      await act(async () => {
        vi.advanceTimersByTime(10_000);
      });
      seen.push(shown(container));
    }
    return seen;
  }

  it("says a summary that has just landed", async () => {
    stubRail([summary(2 * MINUTE)]);
    const { container } = mountRail();
    await waitFor(() => expect(shown(container)).toBe(SUMMARY_LINE));
  });

  // The failure this whole change was opened for, at the surface a reader sees
  // it on: the same run, five hours older, and the rail has stopped saying it.
  //
  // The FRESH run beside it is what makes the absence mean anything. Waiting for
  // the all-clear on its own would pass before the feed had answered at all —
  // "nothing has come back yet" and "what came back is too old to say" render
  // identically — so the case waits for a sentence only the answered feed can
  // produce, and then walks the whole rotation looking for the stale one.
  it("has stopped saying the same summary five hours later", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    stubRail([settledRun("morning_brief", 10 * MINUTE), summary(5 * HOUR)]);
    const { container } = mountRail();
    await waitFor(() => expect(shown(container)).toBe(BRIEF_LINE));
    expect(await walk(container, 4)).not.toContain(SUMMARY_LINE);
  });

  // A quiet installation says only the all-clear: no standing advice joins the
  // rotation while the reader works.
  it("holds the all-clear when there is no news", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    stubRail([]);
    const { container } = mountRail();
    await waitFor(() =>
      expect(shown(container)).toBe(en["agent.line.allClear"]),
    );
    const later = (await walk(container, 3)).slice(1);
    expect(later).toEqual(Array(3).fill(en["agent.line.allClear"]));
  });
});
