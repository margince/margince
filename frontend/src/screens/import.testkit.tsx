// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  type RenderResult,
  render as rtlRender,
  screen,
} from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { ImportCard } from "./import";
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
  run_id: "019ff-draft",
  source_ref: "ws/import/abc",
  object: "lead",
  file_name: "leads.csv",
  rows_profiled: 4,
  row_count: 4,
  columns: [
    { header: "Email", fill_rate: 1, samples: ["ada@x.test", "grace@x.test"] },
    { header: "Full Name", fill_rate: 0.75, samples: ["Ada Lovelace"] },
    { header: "Notes", fill_rate: 0.25, samples: [] },
  ],
  suggested_mapping: { Email: "email", "Full Name": "full_name" },
  targets: ["full_name", "email", "title", "company_name"],
};

// A ZIP of two files the server detected, each with the mapping it proposes for
// every type it could be.
export const zipProfile = {
  run_id: "019ff-draft",
  source_ref: "ws/import/zip",
  file_name: "pipedrive-export.zip",
  suggested_source_name: "pipedrive-export",
  rows_profiled: 0,
  row_count: 5,
  columns: [],
  suggested_mapping: {},
  targets: [],
  parts: [
    {
      file: "companies.csv",
      object: "company",
      rows_profiled: 2,
      row_count: 2,
      columns: [
        { header: "ref", fill_rate: 1, samples: ["CO-1"] },
        { header: "display_name", fill_rate: 1, samples: ["Acme"] },
        { header: "Level", fill_rate: 0.5, samples: ["A"] },
      ],
      suggested_mappings: {
        company: { ref: "ref", display_name: "display_name" },
        contact: { ref: "ref" },
        lead: {},
      },
    },
    {
      file: "contacts.csv",
      object: "contact",
      rows_profiled: 3,
      row_count: 3,
      columns: [
        { header: "ref", fill_rate: 1, samples: ["P-1"] },
        { header: "email", fill_rate: 1, samples: ["jo@acme.com"] },
        { header: "company_ref", fill_rate: 1, samples: ["CO-1"] },
      ],
      suggested_mappings: {
        company: { ref: "ref" },
        contact: { ref: "ref", email: "email", company_ref: "company_ref" },
        lead: { email: "email" },
      },
    },
  ],
  ignored: [{ file: "products.csv", reason: "unknown_name" }],
};

export const run = {
  id: "019ff-run",
  connector: "csv",
  object: "lead",
  status: "awaiting_approval",
  checkpoint: 0,
  version: 3,
  source_released: false,
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
// only the request it names.
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
  "POST /imports": () => jsonResponse({ ...run, status: "validating" }, 202),
};

const RUN_PATH = /^GET \/imports\/([^/]+)$/;

// The answers for a run named by its id.
function runDefault(key: string): Response | undefined {
  const read = RUN_PATH.exec(key);
  if (read) return jsonResponse({ ...run, id: read[1] });
  if (key.startsWith("POST /imports/") && key.endsWith("/approve")) {
    return jsonResponse({ ...run, status: "complete" }, 202);
  }
  if (key.startsWith("PATCH /imports/")) {
    return jsonResponse({
      ...run,
      status: "pending",
      version: run.version + 1,
    });
  }
  if (key.startsWith("POST /imports/") && key.endsWith("/validate")) {
    return jsonResponse(
      { ...run, status: "validating", version: run.version + 1 },
      202,
    );
  }
  if (key.startsWith("GET /imports/") && key.endsWith("/report")) {
    return jsonResponse(dryRun);
  }
  return undefined;
}

// First a route with the same key, the test's or the default's. Then the
// default for a run named by its id. Last, a test route that is a prefix.
// So a stub "GET /imports" does not answer "GET /imports/format". Nor does
// "GET /imports/r1" answer "GET /imports/r1/report".
function answer(routes: Routes, key: string): Response {
  const exact = routes[key] ?? EXACT_DEFAULTS[key];
  if (exact) return exact();
  const byRun = runDefault(key);
  if (byRun) return byRun;
  const prefix = Object.keys(routes).find((candidate) =>
    key.startsWith(candidate),
  );
  const byPrefix = prefix === undefined ? undefined : routes[prefix];
  return byPrefix ? byPrefix() : jsonResponse({});
}

// Every request the screens make, recorded, so a test can assert what went to
// the server. The mapping and the parts are what the screen composes.
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

const RUN_PAGE = "#/settings/import/run";

/**
 * The run page at the test step, reached the way a reader reaches it. A CSV is
 * dropped, its type and map kept, and a test run staged. The staged run
 * is whatever the "POST /imports" route answers (`run` by default).
 */
export async function renderAtTestRun(user: UserEvent): Promise<void> {
  globalThis.location.hash = RUN_PAGE;
  render(<ImportCard subpage="run" />);
  await user.upload(
    await screen.findByLabelText("Upload your data"),
    new File(["Email\nada@x.test\n"], "leads.csv"),
  );
  await user.click(await screen.findByRole("button", { name: /Next: map/ }));
  await user.click(screen.getByRole("button", { name: "Run test" }));
}

/** The run page opened on one run by its address, as Recent imports opens it. */
export function renderAtImport(id: string): RenderResult {
  globalThis.location.hash = `${RUN_PAGE}?run=${id}`;
  return render(<ImportCard subpage="run" />);
}
