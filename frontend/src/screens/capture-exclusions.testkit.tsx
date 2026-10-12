// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The capture-exclusions card's shared harness.
//
// Its own module because two suites drive the same card — the rules it lists
// and the folder picker it opens — and a second copy of this backend would let
// them disagree about what the server answers while both stayed green.
//
// `backend` BUILDS the mock and does not install it: every case stubs fetch
// itself, so one that needs a different answer wraps this one rather than
// editing a shared stub out from under its neighbours.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";

// The two scopes this card draws are two different permissions, and that is the
// whole subject: a rule that binds only the reader is theirs to write, while one
// that binds the company is admin/ops work. So every case below fixes the
// grant and asks what the card offers.
export const CAPTURE_EDITOR: GrantSpec = {
  capture_settings: ["read", "update"],
};
export const READER: GrantSpec = { capture_settings: ["read"] };

export const RULES = [
  { id: "cx-1", scope: "user", kind: "address", value: "ex@partner.test" },
  { id: "cx-2", scope: "workspace", kind: "domain", value: "recruiter.test" },
];

export type Call = { method: string; url: string; body: unknown };

// json answers one route, which is the only shape this mock ever returns.
function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// routeOf answers the routes this card touches, or null for the rule list the
// caller falls through to. Its own function so the mock reads as a routing
// table rather than as one long branch — and so the fixture for each route sits
// beside the URL that asks for it.
function routeOf(
  url: string,
  method: string,
  allow: GrantSpec,
  connectedProvider: string,
): Response | null {
  if (url.endsWith("/v1/me")) {
    return json(meFixture({ allow }));
  }
  // The seat's own connections: the picker asks WHICH mailbox's folders it is
  // offering, so a fixture without one offers nothing.
  if (url.includes("/connectors") && !url.includes("/containers")) {
    return json({
      data: [
        {
          id: "conn-1",
          provider: connectedProvider,
          status: "connected",
          scopes: [],
        },
      ],
      providers: [],
    });
  }
  if (url.includes("/containers")) {
    return json({
      containers: url.includes("/graph/")
        ? [{ id: "AAMk-privat", name: "Posteingang/Privat" }]
        : [{ id: "Label_7", name: "Privat" }],
    });
  }
  if (method === "POST") {
    return json({ id: "cx-3" }, 201);
  }
  return null;
}

export function backend(
  allow: GrantSpec,
  rules: unknown[] = RULES,
  connectedProvider = "gmail",
) {
  const calls: Call[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      // Built as a Request rather than read off `init`: openapi-fetch may pass a
      // Request with no init at all, and a mock that read the method from init
      // would answer every write as if it were a read.
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = request.url;
      const method = request.method;
      calls.push({
        method,
        url,
        body: method === "GET" ? undefined : await request.clone().json(),
      });
      const routed = routeOf(url, method, allow, connectedProvider);
      if (routed !== null) {
        return routed;
      }
      return new Response(JSON.stringify({ data: rules }), {
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  return { fetchMock, calls };
}

export function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

// The header verb that opens the form, named by its catalog key: the dialog's
// own submit says "Exclude", so matching on that would find the wrong control.
export const openForm = () =>
  screen.getByRole("button", { name: en["captureExclusions.addOpen"] });

export const removeVerb = (value: string) =>
  screen.getByRole("button", {
    name: en["captureExclusions.remove"].replace("{value}", value),
  }) as HTMLButtonElement;
