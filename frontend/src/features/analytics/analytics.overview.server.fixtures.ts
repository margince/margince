// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { overviewRoutes } from "./analytics.overview.routes.fixtures";
import type { OverviewSetup } from "./analytics.overview.scenarios.fixtures";

/**
 * Serves a setup to the app: its routes, and each evaluation answered by the
 * request it is, so the movement read gets its own answer.
 */
export function installOverviewServer(setup: OverviewSetup): void {
  installFetchStub(overviewRoutes(setup.routes));
  const routed = globalThis.fetch;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(
      request ? request.url : String(input),
      "http://localhost",
    );
    if (url.pathname.endsWith("/analytics/evaluate"))
      return Promise.resolve(jsonResponse(setup.answer(url)));
    // The returned list is the waiting route with a state; the stub's route
    // map cannot see a query, so it is answered here.
    if (
      url.pathname.endsWith("/analytics/sdr-summary/handoffs") &&
      url.searchParams.get("state") === "returned"
    )
      return Promise.resolve(
        setup.returned?.() ??
          jsonResponse({ as_of: url.searchParams.get("as_of"), rows: [] }),
      );
    return routed(input, init);
  }) as typeof fetch;
}
