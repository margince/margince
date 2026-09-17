// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import { locationDouble } from "../testing/locationdouble";

// What every sign-in suite needs around the screen: the query client the
// capability probe runs in, an English locale, and a stubbed
// /auth/capabilities — the probe that decides which methods the screen draws,
// so no case about this surface can avoid answering it.
//
// Named `.testkit.` rather than `.test.` for the reason the record-shell kit
// is: it holds no cases of its own, and the suffix is what tells the file-shape
// gates it is not a suite.

/** The English catalog, which is what the assertions in these suites read. */
export const t = (key: Parameters<typeof translate>[1]) => translate("en", key);

export const render = (ui: ReactNode): RenderResult => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

export // stubApi answers GET /auth/capabilities from `capabilities` and records
// every other call for the test to assert on.
//
// `oidc_providers` defaults to [] — the running installation's own answer while
// the OIDC flow has not shipped (§19), and what keeps every case below asserting
// a surface with no federated block. A test that wants one passes it.
function stubApi(
  capabilities: {
    /** Omitted where a case is about a probe that does not carry it. */
    password?: boolean;
    password_reset: boolean;
    oidc_providers?: ReadonlyArray<{ key: string; label: string }>;
  },
  respond: (request: Request) => Response | Promise<Response>,
) {
  const calls: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      if (new URL(request.url).pathname.endsWith("/auth/capabilities")) {
        return new Response(
          JSON.stringify({ oidc_providers: [], ...capabilities }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      calls.push(request);
      return respond(request);
    }),
  );
  return calls;
}

export const ok = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

// stubLocationAssign swaps `window.location` for the duration of `run`, so a
// test can observe `location.assign` calls without a real cross-origin
// navigation. `Location.prototype.assign` is non-configurable in jsdom, so
// `vi.spyOn` cannot touch it — the whole object has to move.
export async function stubLocationAssign(
  run: (assign: ReturnType<typeof vi.fn>) => Promise<void>,
) {
  const originalLocation = window.location;
  const assign = vi.fn();
  Object.defineProperty(window, "location", {
    value: locationDouble({ assign }),
    writable: true,
    configurable: true,
  });
  try {
    await run(assign);
  } finally {
    Object.defineProperty(window, "location", {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
  }
}
