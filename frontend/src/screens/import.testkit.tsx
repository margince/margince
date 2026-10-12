// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { formatFixture } from "./import.fixtures";

// The import screens' test harness: one fetch stub that records what was sent,
// the providers every screen needs, and the wire fixtures the files share.

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function render(ui: ReactNode): RenderResult {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

export const profile = {
  source_ref: "ws/import/abc",
  object: "lead",
  rows_profiled: 4,
  columns: [
    { header: "Email", fill_rate: 1, samples: ["ada@x.test", "grace@x.test"] },
    { header: "Full Name", fill_rate: 0.75, samples: ["Ada Lovelace"] },
    { header: "Notes", fill_rate: 0.25, samples: [] },
  ],
  suggested_mapping: { Email: "email", "Full Name": "full_name" },
  targets: ["full_name", "email", "title", "company_name"],
};

export const run = {
  id: "019ff-run",
  connector: "csv",
  object: "lead",
  status: "awaiting_approval",
  checkpoint: 0,
  source: "import_api",
  captured_by: "u-ada",
  created_at: "2026-08-13T10:00:00Z",
  updated_at: "2026-08-13T10:00:00Z",
};

export const dryRun = {
  run_id: run.id,
  status: "awaiting_approval",
  rows_read: 4,
  disposition: { created: 3, updated: 0, unchanged: 0, skipped: 1 },
  issues: [{ line: 3, reason: 'the "Email" column is empty' }],
  source_key_used: "Email",
};

export { formatFixture };

export type Sent = { method: string; path: string; body?: unknown };

type Routes = Record<string, () => Response>;

// The answers every import screen reads before a test says otherwise, each for
// exactly the request it names.
const EXACT_DEFAULTS: Routes = {
  "GET /me": () =>
    jsonResponse(
      meFixture({
        roles: ["admin"],
        allow: { import_run: ["create", "update", "read"] },
      }),
    ),
  "GET /imports/format": () => jsonResponse(formatFixture),
  "GET /imports": () => jsonResponse({ data: [], page: { has_more: false } }),
  "GET /users/names": () => jsonResponse({ data: [] }),
  "POST /imports/sources": () => jsonResponse(profile),
  "POST /imports": () => jsonResponse(run, 202),
};

// The answers for a run named by its id.
function runDefault(key: string): Response {
  if (key.startsWith("POST /imports/") && key.endsWith("/approve")) {
    return jsonResponse({ ...run, status: "complete" }, 202);
  }
  if (key.endsWith("/report")) return jsonResponse(dryRun);
  return jsonResponse({});
}

// A request is answered by the route that names it exactly, the test's or the
// default's, before any route answers it by prefix: a stub "GET /imports" does
// not answer "GET /imports/format", and "GET /imports/r1" does not answer
// "GET /imports/r1/report".
function answer(routes: Routes, key: string): Response {
  const exact = routes[key] ?? EXACT_DEFAULTS[key];
  if (exact) return exact();
  const prefix = Object.keys(routes).find((candidate) =>
    key.startsWith(candidate),
  );
  const byPrefix = prefix === undefined ? undefined : routes[prefix];
  return byPrefix ? byPrefix() : runDefault(key);
}

// Every request the screens make, recorded, so a test can assert what went to
// the server: the mapping and the parts are what the screen composes.
export function stubRoutes(routes: Routes = {}): Sent[] {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      // The typed client calls fetch with a Request; the multipart upload
      // calls it with a url and an init.
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      sent.push({ method, path: key, body: await bodyOf(request, init) });
      return answer(routes, key);
    }),
  );
  return sent;
}

async function bodyOf(
  request: Request | null,
  init: RequestInit | undefined,
): Promise<unknown> {
  const raw = request ? await request.clone().text() : init?.body;
  if (typeof raw === "string" && raw.length > 0) {
    return JSON.parse(raw);
  }
  if (init?.body instanceof FormData) {
    return Object.fromEntries(
      [...init.body.entries()].map(([k, v]) => [
        k,
        v instanceof File ? v.name : v,
      ]),
    );
  }
  return undefined;
}
