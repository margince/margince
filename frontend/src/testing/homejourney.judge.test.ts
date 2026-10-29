// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { MAX_RETRIES } from "../app/queryclient";
import {
  type Answer,
  budgetSamples,
  degradedReasons,
  expectationFrom,
  type Frame,
  framesLostAfterReturn,
  type Journey,
  longestLoss,
  outcomeOf,
  QUERY_RETRIES,
  RequestLedger,
  readerFrom,
  readOf,
  tally,
  withheldSources,
} from "./homejourney.judge";
import { BUSY_NIGHT } from "./magic.fixtures";
import { day, row } from "./worklist.testkit";

const READER = "reader-1";
const WORK = expectationFrom("work");
const NONE = expectationFrom("none");
const FOCUS = { items: [], total: 0, urgent_remaining: 0 };
const busyDay = day({
  queue: [row({ id: "task-1" })],
  focus: { items: [row({ id: "task-1" })], total: 1, urgent_remaining: 0 },
});

function answer(over: Partial<Answer> & Pick<Answer, "read">): Answer {
  return { journey: "load-1", status: 200, body: {}, ...over };
}

/** A healthy answer for every Home read, with `over` replacing its read's. */
function healthy(...over: Answer[]): Answer[] {
  const base: Answer[] = [
    answer({ read: "worklist", body: busyDay }),
    answer({ read: "handled", body: { receipts: [] } }),
    answer({ read: "brief", status: 404, body: { code: "not_found" } }),
    answer({ read: "magic", body: BUSY_NIGHT }),
    answer({ read: "me", body: { user: { id: READER } } }),
  ];
  return base.map((a) => over.find((o) => o.read === a.read) ?? a);
}

describe("expectationFrom", () => {
  it("reads a comma list and refuses a word it does not know", () => {
    expect(expectationFrom("work, brief")).toEqual({
      work: true,
      brief: true,
      handled: false,
    });
    expect(() => expectationFrom("work,briefs")).toThrow(/briefs/);
    expect(() => expectationFrom("none,work")).toThrow(/none/);
  });
});

describe("readOf", () => {
  const at = (path: string) => new URL(`http://localhost${path}`);

  it("names only the reads Home's first page makes", () => {
    expect(readOf(at("/v1/worklist?scope=mine&filter=all"), "GET")).toBe(
      "worklist",
    );
    expect(readOf(at("/v1/worklist?scope=team&filter=all"), "GET")).toBe(
      undefined,
    );
    expect(readOf(at("/v1/worklist?owner=u&filter=all"), "GET")).toBe(
      undefined,
    );
    expect(readOf(at("/v1/worklist/handled?cursor=c"), "GET")).toBe(undefined);
    expect(readOf(at("/v1/brief"), "POST")).toBe(undefined);
    expect(readOf(at("/v1/magic?limit=100"), "GET")).toBe("magic");
  });
});

describe("degradedReasons", () => {
  it("finds nothing wrong with a whole day for the signed-in reader", () => {
    expect(degradedReasons(healthy(), WORK, READER)).toEqual([]);
  });

  it("names a read that never answered", () => {
    const answers = healthy().filter((a) => a.read !== "magic");
    expect(degradedReasons(answers, WORK, READER)).toEqual([
      "magic: no answer",
    ]);
  });

  it("names a malformed body and a partial day", () => {
    const answers = healthy(
      answer({ read: "worklist", body: { scope: "mine", queue: [] } }),
      answer({ read: "magic", body: undefined }),
    );
    expect(degradedReasons(answers, WORK, READER)).toEqual([
      "worklist: answered a partial day",
      "magic: answered 200 with no JSON body",
    ]);
  });

  it("keeps a failed first answer against the page after a good retry", () => {
    const answers = [
      ...healthy(),
      answer({
        read: "worklist",
        status: 503,
        body: { code: "service_unavailable" },
      }),
    ];
    expect(degradedReasons(answers, WORK, READER)).toEqual([
      "worklist: answered 503",
    ]);
  });

  it("names a failed source on the day and on the receipt", () => {
    const failed = [{ source: "email", reason: "failed" as const }];
    const answers = healthy(
      answer({
        read: "worklist",
        body: { ...busyDay, sources_unavailable: failed },
      }),
      answer({
        read: "magic",
        body: { ...BUSY_NIGHT, sources_unavailable: failed },
      }),
    );
    expect(degradedReasons(answers, WORK, READER)).toEqual([
      "worklist: source email failed",
      "magic: source email failed",
    ]);
  });

  it("reads an empty day as degraded only for a reader who has work", () => {
    const empty = healthy(
      answer({ read: "worklist", body: day({ focus: FOCUS }) }),
    );
    expect(degradedReasons(empty, WORK, READER)).toEqual([
      "worklist: empty queue for a reader who has work",
      "worklist: no focus item for a reader who has work",
    ]);
    expect(degradedReasons(empty, NONE, READER)).toEqual([]);
  });

  it("names a session answered for somebody else", () => {
    const answers = healthy(
      answer({ read: "me", body: { user: { id: "reader-2" } } }),
    );
    expect(degradedReasons(answers, WORK, READER)).toEqual([
      "me: answered for another reader",
    ]);
  });

  it("requires a brief once the reader is expected to have one", () => {
    expect(
      degradedReasons(healthy(), expectationFrom("work,brief"), READER),
    ).toEqual(["brief: answered 404"]);
  });

  it("lists a withheld source without counting it against the page", () => {
    const withheld = healthy(
      answer({
        read: "worklist",
        body: {
          ...busyDay,
          sources_unavailable: [{ source: "deal", reason: "withheld" }],
        },
      }),
    );
    expect(degradedReasons(withheld, WORK, READER)).toEqual([]);
    expect(withheldSources(withheld)).toEqual(["worklist:deal"]);
  });
});

describe("readerFrom", () => {
  it("names the reader and the name Home greets them by", () => {
    expect(
      readerFrom({ user: { id: READER, display_name: "Demo Admin" } }),
    ).toEqual({ id: READER, greeting: "Demo" });
    expect(() => readerFrom({ user: {} })).toThrow(/no user id/);
  });
});

describe("frames", () => {
  const frame = (at: number, shown: boolean, reader = true): Frame => ({
    at,
    day: shown,
    reader,
  });

  it("measures the longest stretch without the day or its reader", () => {
    expect(longestLoss([frame(0, true), frame(16, true)])).toBe(0);
    expect(
      longestLoss([
        frame(0, true),
        frame(16, false),
        frame(32, true, false),
        frame(48, false),
        frame(64, true),
      ]),
    ).toBe(32);
  });

  it("counts losses only after the day came back", () => {
    expect(
      framesLostAfterReturn([
        frame(0, false),
        frame(16, true),
        frame(32, true),
      ]),
    ).toEqual([]);
    expect(
      framesLostAfterReturn([frame(0, true), frame(16, false)]),
    ).toHaveLength(1);
    expect(framesLostAfterReturn([frame(0, false)])).toHaveLength(1);
  });
});

describe("outcomes and budgets", () => {
  const journey = (outcome: Journey["outcome"], useful?: number): Journey => ({
    outcome,
    times: useful === undefined ? {} : { useful },
    reasons: outcome === "complete" ? [] : ["worklist: answered 503"],
    withheld: [],
  });

  it("never reads a journey with gaps as complete", () => {
    const marks = { useful: true, failed: false };
    expect(outcomeOf(marks, true, [])).toBe("complete");
    expect(outcomeOf(marks, true, ["magic: no answer"])).toBe("degraded");
    expect(outcomeOf(marks, false, [])).toBe("useful");
    expect(outcomeOf({ useful: false, failed: false }, false, [])).toBe(
      "timed out",
    );
    expect(outcomeOf({ useful: true, failed: true }, true, [])).toBe("failed");
  });

  it("counts a degraded or unreached journey as never against a budget", () => {
    expect(
      budgetSamples(
        [journey("complete", 30), journey("degraded", 20), journey("complete")],
        "useful",
      ),
    ).toEqual([30, Number.POSITIVE_INFINITY, Number.POSITIVE_INFINITY]);
  });

  it("tallies outcomes and reasons", () => {
    const { outcomes, reasons } = tally([
      journey("complete", 1),
      journey("degraded", 1),
      journey("degraded", 1),
    ]);
    expect([...outcomes]).toEqual([
      ["complete", 1],
      ["degraded", 2],
    ]);
    expect([...reasons]).toEqual([["worklist: answered 503", 2]]);
  });
});

describe("RequestLedger", () => {
  it("is the query client's retry count", () => {
    expect(QUERY_RETRIES).toBe(MAX_RETRIES);
  });

  it("charges a retry to the journey of the request it repeats", () => {
    const ledger = new RequestLedger();
    ledger.begin("refresh-1");
    const first = ledger.started("worklist");
    ledger.finished("worklist", first, 503);
    expect(ledger.busy("refresh-1")).toBe(true);
    ledger.begin("refresh-2");
    const retry = ledger.started("worklist");
    expect(retry).toBe("refresh-1");
    ledger.finished("worklist", retry, 200);
    expect(ledger.busy("refresh-1")).toBe(false);
    expect(ledger.started("worklist")).toBe("refresh-2");
  });

  it("stops waiting once the client has spent its retries", () => {
    const ledger = new RequestLedger();
    ledger.begin("load-1");
    for (let attempt = 0; attempt <= QUERY_RETRIES; attempt++) {
      ledger.finished("magic", ledger.started("magic"), 500);
    }
    expect(ledger.busy("load-1")).toBe(false);
  });

  it("expects no retry after a refusal or a lost connection", () => {
    const ledger = new RequestLedger();
    ledger.begin("load-1");
    ledger.finished("brief", ledger.started("brief"), 404);
    ledger.finished("handled", ledger.started("handled"), null);
    expect(ledger.busy("load-1")).toBe(false);
  });

  it("gives up on a retry that never came when released", () => {
    const ledger = new RequestLedger();
    ledger.begin("load-1");
    ledger.finished("magic", ledger.started("magic"), 501);
    ledger.release("load-1");
    expect(ledger.busy("load-1")).toBe(false);
  });
});
