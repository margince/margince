// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  elapsedPhase,
  type RequestMeasurement,
  readinessPhases,
  requestPhases,
} from "./homephases.judge";

const request: RequestMeasurement = {
  read: "worklist",
  journey: "reload",
  status: 200,
  failed: false,
  timing: {
    startTime: 80,
    requestStart: 120,
    responseStart: 240,
    responseEnd: 260,
  },
  sendEnd: 130,
  anchorMs: 0,
};

describe("Home request phase attribution", () => {
  it("keeps application delay, browser pre-send, sending and waiting separate", () => {
    expect(requestPhases(request)).toEqual({
      beforeFetchMs: 80,
      browserPreSendMs: 40,
      dnsMs: null,
      connectMs: null,
      tlsMs: null,
      sendMs: 10,
      waitingMs: 110,
      receiveMs: 20,
      totalMs: 180,
    });
  });

  it("attributes a delayed answer to waiting without extending pre-send", () => {
    const slow = requestPhases({
      ...request,
      timing: { ...request.timing, responseStart: 740, responseEnd: 760 },
    });
    expect(slow.waitingMs).toBe(610);
    expect(slow.browserPreSendMs).toBe(40);
    expect(slow.receiveMs).toBe(20);
  });

  it("does not invent send or waiting durations when send end is unavailable", () => {
    const phases = requestPhases({ ...request, sendEnd: null });
    expect(phases.sendMs).toBeNull();
    expect(phases.waitingMs).toBeNull();
    expect(phases.totalMs).toBe(180);
  });

  it.each([
    [-1, 10],
    [10, -1],
    [20, 10],
    [Number.NaN, 10],
    [0, Number.POSITIVE_INFINITY],
    [null, 10],
  ])("retains unavailable or invalid phase %s → %s", (from, until) => {
    expect(elapsedPhase(from, until)).toBeNull();
  });

  it("keeps a genuine zero interval", () => {
    expect(elapsedPhase(10, 10)).toBe(0);
  });

  it("separates readiness after concurrent responses from request total", () => {
    expect(readinessPhases([100, 160, 120], 190, 600, false)).toEqual({
      usefulSinceDocumentMs: 190,
      completeSinceDocumentMs: 600,
      afterResponseMs: 440,
    });
  });

  it("preserves retained useful content before a refetch finishes", () => {
    expect(readinessPhases([180], 20, 200, false)).toEqual({
      usefulSinceDocumentMs: 20,
      completeSinceDocumentMs: 200,
      afterResponseMs: 20,
    });
  });

  it("does not report a failed journey as fast readiness", () => {
    expect(readinessPhases([100], 20, 110, true)).toEqual({
      usefulSinceDocumentMs: null,
      completeSinceDocumentMs: null,
      afterResponseMs: null,
    });
  });

  it("does not truncate the response census when one timing is missing", () => {
    expect(
      readinessPhases([100, -1], 150, 180, false).afterResponseMs,
    ).toBeNull();
  });
});
