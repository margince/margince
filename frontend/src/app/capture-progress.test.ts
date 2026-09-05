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
): BackfillStatus {
  return {
    state,
    estimated_messages: estimated,
    counts: { messages_scanned: scanned },
  };
}

describe("liveCapture", () => {
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
