// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";

export const MCP_URL = "https://crm.acme.test/mcp";

export const ADMIN_GRANTS: GrantSpec = {
  agent_client: ["create", "read", "update", "delete"],
  installation_settings: ["read", "update"],
};

export type Call = Readonly<{
  key: string;
  search: string;
  body: unknown;
}>;

type Answer = (call: Call) => Response | Promise<Response>;

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * Answers each "METHOD /path" with its route, `/v1` dropped, and records every
 * call, so a case reads what the card sent. Discovery answers the MCP URL
 * unless a route says otherwise.
 */
export function serve(
  routes: Readonly<Record<string, Answer>>,
  allow: GrantSpec = ADMIN_GRANTS,
): Call[] {
  const calls: Call[] = [];
  const all: Record<string, Answer> = {
    "GET /me": () => json(meFixture({ allow })),
    "GET /.well-known/oauth-protected-resource": () =>
      json({ resource: MCP_URL }),
    ...routes,
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request
          ? input
          : new Request(new URL(String(input), "https://crm.acme.test"), init);
      const url = new URL(request.url);
      const key = `${request.method} ${url.pathname.replace(/^\/v1/, "")}`;
      const text = request.method === "GET" ? "" : await request.text();
      const call = {
        key,
        search: url.search,
        body: text === "" ? null : JSON.parse(text),
      };
      calls.push(call);
      const answer = all[key];
      return answer ? answer(call) : json({ title: "Not Found" }, 404);
    }),
  );
  return calls;
}

export function renderWithClient(ui: ReactNode): RenderResult {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

export function validationProblem(field: string, code: string): Response {
  return json(
    {
      title: "Unprocessable",
      status: 422,
      code: "validation_error",
      details: { errors: [{ field, code, message: `${field}: ${code}` }] },
    },
    422,
  );
}
