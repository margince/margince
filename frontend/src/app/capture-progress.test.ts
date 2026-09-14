// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { connectorsPollInterval, liveCapture } from "./capture-progress";

type CaptureConnection = components["schemas"]["CaptureConnection"];
type BackfillStatus = components["schemas"]["BackfillStatus"];

// What the chrome reads off the connections list: whether mail is arriving,
// how far along, and from where. Every figure is the server's own row count;
// nothing here may invent an import, a share or a source.

function mailbox(
  over: Partial<CaptureConnection> & { backfill?: BackfillStatus },
): CaptureConnection {
  return {
    id: "018f3a1b-0000-7000-8000-0000000000c1",
    provider: "gmail",
    status: "connected",
    scopes: [],
    account_label: "ada@acme.test",
    ...over,
  };
}

function run(
  state: BackfillStatus["state"],
  scanned: number,
  estimated: number | null,
  floor = false,
): BackfillStatus {
  return {
    state,
    estimated_messages: estimated,
    estimate_is_floor: floor,
    counts: { messages_scanned: scanned },
  };
}

describe("liveCapture", () => {
  // A floor denominator is a bound, not a total: the provider stopped counting
  // at its cap, so the window holds at least that many and how many more is
  // unknown. Under it, the share is an upper bound and worth drawing; past it,
  // clamping to 1 would say the import is COMPLETE when nothing here knows how
  // much is left — so the ring goes and the counts stand on their own.
  it("drops the share once a floor denominator is passed, rather than pinning it full", () => {
    const under = liveCapture([
      mailbox({ backfill: run("running", 5_000, 20_000, true) }),
    ]);
    expect(under?.estimatedIsFloor).toBe(true);
    expect(under?.fraction).toBe(0.25);

    const past = liveCapture([
      mailbox({ backfill: run("running", 24_000, 20_000, true) }),
    ]);
    expect(past?.estimatedIsFloor).toBe(true);
    expect(past?.fraction).toBeNull();
    // The counts are still the truth, and they are what the surfaces fall
    // back to.
    expect(past?.scanned).toBe(24_000);
    expect(past?.estimated).toBe(20_000);
  });

  // An EXACT denominator that is overrun is a different fact — the mailbox grew
  // between the preview and the scan — and the clamp is the right answer there:
  // the run is genuinely at its end.
  it("still clamps an exact denominator the scan overran", () => {
    const over = liveCapture([
      mailbox({ backfill: run("running", 1_200, 1_000) }),
    ]);
    expect(over?.estimatedIsFloor).toBe(false);
    expect(over?.fraction).toBe(1);
  });

  // One capped mailbox makes the SUM a bound: the other mailbox's exact count
  // says nothing about how much the capped one still holds.
  it("treats a sum containing one floor as a floor", () => {
    const both = liveCapture([
      mailbox({ backfill: run("running", 100, 500) }),
      mailbox({
        id: "018f3a1b-0000-7000-8000-0000000000c2",
        account_label: "grace@acme.test",
        backfill: run("running", 21_000, 20_000, true),
      }),
    ]);
    expect(both?.estimatedIsFloor).toBe(true);
    // 21,100 scanned against a 20,500 bound: past it, so no share.
    expect(both?.fraction).toBeNull();
  });

  it("is null when no mailbox is importing, never a zero-progress reading", () => {
    expect(liveCapture([])).toBeNull();
    expect(liveCapture([mailbox({})])).toBeNull();
    expect(liveCapture([mailbox({ backfill: { state: "none" } })])).toBeNull();
    expect(
      liveCapture([mailbox({ backfill: run("done", 900, 900) })]),
    ).toBeNull();
    expect(
      liveCapture([mailbox({ backfill: run("cancelled", 12, 900) })]),
    ).toBeNull();
    expect(
      liveCapture([mailbox({ backfill: run("error", 12, 900) })]),
    ).toBeNull();
  });

  it("reads a running import's share from the persisted counts", () => {
    expect(
      liveCapture([mailbox({ backfill: run("running", 420, 1_000) })]),
    ).toEqual({
      scanned: 420,
      estimated: 1_000,
      estimatedIsFloor: false,
      fraction: 0.42,
      sources: ["ada@acme.test"],
    });
  });

  it("counts a queued import as live: the work has been taken up", () => {
    expect(
      liveCapture([mailbox({ backfill: run("queued", 0, 1_000) })]),
    ).toEqual({
      scanned: 0,
      estimated: 1_000,
      estimatedIsFloor: false,
      fraction: 0,
      sources: ["ada@acme.test"],
    });
  });

  it("carries no fraction without a denominator, rather than guessing one", () => {
    expect(
      liveCapture([mailbox({ backfill: run("running", 37, null) })]),
    ).toEqual({
      scanned: 37,
      estimated: null,
      estimatedIsFloor: false,
      fraction: null,
      sources: ["ada@acme.test"],
    });
    // A zero estimate is no denominator either.
    expect(
      liveCapture([mailbox({ backfill: run("running", 37, 0) })])?.fraction,
    ).toBeNull();
  });

  it("clamps a scan that outgrew its preview to a full ring", () => {
    expect(
      liveCapture([mailbox({ backfill: run("running", 1_200, 1_000) })])
        ?.fraction,
    ).toBe(1);
  });

  it("folds two importing mailboxes into one reading and names both", () => {
    const reading = liveCapture([
      mailbox({ backfill: run("running", 300, 1_000) }),
      mailbox({
        id: "018f3a1b-0000-7000-8000-0000000000c2",
        provider: "graph",
        account_label: null,
        backfill: run("queued", 100, 1_000),
      }),
      mailbox({
        id: "018f3a1b-0000-7000-8000-0000000000c3",
        provider: "gcal",
        backfill: run("done", 50, 50),
      }),
    ]);
    expect(reading).toEqual({
      scanned: 400,
      estimated: 2_000,
      estimatedIsFloor: false,
      fraction: 0.2,
      // A mailbox with no label is named by its provider, never dropped.
      sources: ["ada@acme.test", "graph"],
    });
  });

  it("sums only the estimates that exist, so one unpreviewed mailbox does not erase the other's share", () => {
    const reading = liveCapture([
      mailbox({ backfill: run("running", 500, 1_000) }),
      mailbox({
        id: "018f3a1b-0000-7000-8000-0000000000c2",
        backfill: run("running", 20, null),
      }),
    ]);
    expect(reading?.scanned).toBe(520);
    expect(reading?.estimated).toBe(1_000);
    expect(reading?.fraction).toBe(0.52);
  });
});

describe("connectorsPollInterval", () => {
  it("polls live only while a mailbox is importing", () => {
    expect(connectorsPollInterval(undefined)).toBe(false);
    expect(connectorsPollInterval({ data: [mailbox({})] })).toBe(false);
    expect(
      connectorsPollInterval({
        data: [mailbox({ backfill: run("done", 10, 10) })],
      }),
    ).toBe(false);
    expect(
      connectorsPollInterval({
        data: [mailbox({ backfill: run("running", 1, 10) })],
      }),
    ).toBe(2_500);
  });
});
