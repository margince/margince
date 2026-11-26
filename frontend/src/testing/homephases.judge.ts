// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { HomeRead } from "./homejourney.judge";

export type RequestPhases = {
  beforeFetchMs: number | null;
  browserPreSendMs: number | null;
  dnsMs: number | null;
  connectMs: number | null;
  tlsMs: number | null;
  sendMs: number | null;
  waitingMs: number | null;
  receiveMs: number | null;
  totalMs: number | null;
};

export type BrowserRequestTiming = {
  startTime: number;
  requestStart: number;
  responseStart: number;
  responseEnd: number;
  domainLookupStart?: number;
  domainLookupEnd?: number;
  connectStart?: number;
  connectEnd?: number;
  secureConnectionStart?: number;
};

export type RequestMeasurement = {
  read: HomeRead;
  journey: string;
  status: number | null;
  failed: boolean;
  timing: BrowserRequestTiming;
  sendEnd: number | null;
  anchorMs: number;
  intercepted?: boolean;
};

export function elapsedPhase(
  from: number | null,
  until: number | null,
): number | null {
  if (
    from === null ||
    until === null ||
    !Number.isFinite(from) ||
    !Number.isFinite(until) ||
    from < 0 ||
    until < from
  )
    return null;
  return until - from;
}

export function requestPhases(request: RequestMeasurement): RequestPhases {
  const { timing, sendEnd } = request;
  const measured = {
    beforeFetchMs: elapsedPhase(request.anchorMs, timing.startTime),
    browserPreSendMs: elapsedPhase(timing.startTime, timing.requestStart),
    dnsMs: elapsedPhase(
      timing.domainLookupStart ?? null,
      timing.domainLookupEnd ?? null,
    ),
    connectMs: elapsedPhase(
      timing.connectStart ?? null,
      timing.connectEnd ?? null,
    ),
    tlsMs: elapsedPhase(
      timing.secureConnectionStart ?? null,
      timing.connectEnd ?? null,
    ),
    sendMs: elapsedPhase(timing.requestStart, sendEnd),
    waitingMs: elapsedPhase(sendEnd, timing.responseStart),
    receiveMs: elapsedPhase(timing.responseStart, timing.responseEnd),
    totalMs: elapsedPhase(timing.startTime, timing.responseEnd),
  };
  if (!request.intercepted) return measured;
  return {
    beforeFetchMs: measured.beforeFetchMs,
    browserPreSendMs: null,
    dnsMs: null,
    connectMs: null,
    tlsMs: null,
    sendMs: null,
    waitingMs: null,
    receiveMs: null,
    totalMs: null,
  };
}

export function readinessPhases(
  responseEnds: readonly number[],
  usefulAt: number | null,
  completeAt: number | null,
  failed: boolean,
) {
  const available = responseEnds.filter((at) => Number.isFinite(at) && at >= 0);
  const lastResponse =
    available.length === responseEnds.length && available.length > 0
      ? Math.max(...available)
      : null;
  return {
    usefulSinceDocumentMs: failed ? null : elapsedPhase(0, usefulAt),
    completeSinceDocumentMs: failed ? null : elapsedPhase(0, completeAt),
    afterResponseMs: failed ? null : elapsedPhase(lastResponse, completeAt),
  };
}
