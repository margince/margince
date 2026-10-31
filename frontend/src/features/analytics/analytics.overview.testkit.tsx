// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a suite opens the Overview and reads what it asked. The server it meets
// is in analytics.overview.fixtures.ts, which stories import without vitest.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { LocaleProvider } from "../../i18n";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { AnalyticsScreen } from "./analytics";
import {
  isMovement,
  overviewRoutes,
  prospectingEvaluation,
  type Seat,
  salesEvaluation,
  startingMovement,
} from "./analytics.overview.fixtures";
import type { ReportingEvaluation } from "./reporting.model";

export * from "./analytics.overview.fixtures";

type Answer = ReportingEvaluation | Response | Promise<Response>;

export type AskedRequest = {
  readonly url: URL;
  readonly signal: AbortSignal | undefined;
  // The order in which requests started and settled, on one counter, so a
  // case can tell which were in flight together. Unset while still in flight.
  readonly started: number;
  settled?: number;
};

/**
 * The Analytics screen at an address, with each evaluation answered by what
 * it asks. Returns every request in the order it was made, with its signal.
 * `client` is for a case that counts requests: the app's own client decides
 * by its freshness window which mounts read again. A request `hold` names is
 * never answered, so it stays in flight.
 */
export function openOverview(
  address: string,
  answer: (url: URL) => Answer,
  routes: Parameters<typeof overviewRoutes>[0] = {},
  client: QueryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  }),
  hold: (url: URL) => boolean = () => false,
): AskedRequest[] {
  globalThis.location.hash = address;
  installFetchStub(overviewRoutes(routes));
  const routed = globalThis.fetch;
  const asked: AskedRequest[] = [];
  let clock = 0;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(
      request ? request.url : String(input),
      "http://localhost",
    );
    const entry: AskedRequest = {
      url,
      signal: request?.signal ?? init?.signal ?? undefined,
      started: clock++,
    };
    asked.push(entry);
    if (hold(url)) return new Promise<Response>(() => {});
    try {
      const own = ownAnswer(url, answer, routes);
      if (own) return await own;
      return await routed(input, init);
    } finally {
      entry.settled = clock++;
    }
  }) as typeof fetch;
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AnalyticsScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return asked;
}

// The requests the route map cannot answer. An evaluation is answered by what
// it asks; the returned list is the waiting route with a state.
function ownAnswer(
  url: URL,
  answer: (url: URL) => Answer,
  routes: Parameters<typeof overviewRoutes>[0],
): Response | Promise<Response> | undefined {
  if (
    url.pathname.endsWith("/analytics/sdr-summary/handoffs") &&
    url.searchParams.get("state") === "returned"
  )
    return routes?.returned?.();
  if (!url.pathname.endsWith("/analytics/evaluate")) return undefined;
  const given = answer(url);
  return given instanceof Response || given instanceof Promise
    ? given
    : jsonResponse(given);
}

/** The most requests `among` names that were in flight at one moment. */
export function peakInFlight(
  asked: readonly AskedRequest[],
  among: (url: URL) => boolean,
): number {
  const counted = asked.filter(({ url }) => among(url));
  return Math.max(
    0,
    ...counted.map(
      ({ started }) =>
        counted.filter(
          (other) =>
            other.started <= started &&
            (other.settled === undefined || other.settled > started),
        ).length,
    ),
  );
}

/** Each seat's default answer: its view's evaluation, and no second capture. */
export function bySeat(seat: Seat) {
  return (url: URL) => {
    if (isMovement(url)) return startingMovement();
    return seat === "sdr" ? prospectingEvaluation() : salesEvaluation();
  };
}

export async function readings() {
  const strip = await screen.findByRole("region", {
    name: "Opening readings",
  });
  return within(strip);
}
