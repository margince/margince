import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";

// The share screen's fetch stub and fixtures, shared by its test files.

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

export const usersPage = {
  data: [
    { id: "u-1", display_name: "Priya Shah", email: "priya@example.com" },
    { id: "u-2", display_name: "Mor Adler", email: "mor@example.com" },
    // An agent seat, which is never offered as a share subject.
    {
      id: "u-agent",
      display_name: "SDR Bot",
      email: "bot@example.com",
      is_agent: true,
    },
  ],
  page: { next_cursor: null, has_more: false },
};

export const teamsPage = {
  data: [{ id: "t-1", name: "Deal Desk", member_count: 4 }],
  page: { next_cursor: null, has_more: false },
};

export const existingGrant = {
  id: "g-1",
  record_type: "deal",
  record_id: "d-1",
  subject_type: "user" as const,
  subject_id: "u-2",
  access: "read" as const,
  granted_by: "u-1",
  reason: "compliance review",
  expires_at: null,
  created_at: "2026-06-22T14:08:00Z",
  version: 1,
};

// Mor Adler's row in the picker, not the Revoke button that also names her.
export const PICK_MOR = /^(?!Revoke).*Mor Adler/;

// The same subject holding write instead of read: the only starting point
// from which a reduction is possible.
export const writeGrant = { ...existingGrant, access: "write" as const };

// Either level: each fixture fixes its own `access` literal, so a case seeds
// the starting point it is about.
export type SeededGrant = typeof existingGrant | typeof writeGrant;

/**
 * The grants endpoint with the POST bodies collected.
 *
 * `listed` is the grant the record already carries, so the screen knows what a
 * re-assert is measured against. The POST answers with the restated row, as an
 * idempotent create does.
 */
export function grantEndpoint(
  posts: Record<string, unknown>[],
  listed: SeededGrant,
) {
  return (request: Request) => {
    if (request.method === "POST") {
      return request.json().then((body: Record<string, unknown>) => {
        posts.push(body);
        return jsonResponse({ ...listed, ...body }, 201);
      });
    }
    return jsonResponse({
      data: [listed],
      page: { next_cursor: null, has_more: false },
    });
  };
}

// A seat that may share every record kind: sharing asks the record's update.
export const SHARER = {
  rowScope: "all",
  allow: {
    contact: ["read", "update"],
    company: ["read", "update"],
    deal: ["read", "update"],
    lead: ["read", "update"],
    project: ["read", "update"],
  },
} as const satisfies Parameters<typeof meFixture>[0];

// The record read the screen makes before it offers a share.
export const RECORD_READ =
  /^\/v1\/(contacts|companies|deals|leads|projects)\/[^/]+$/;

export function installBaseFetch(
  overrides: Record<
    string,
    (req: Request) => Response | Promise<Response>
  > = {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      for (const [match, handler] of Object.entries(overrides)) {
        if (request.url.includes(match)) {
          return handler(request);
        }
      }
      return baseAnswer(request);
    }),
  );
}

function baseAnswer(request: Request): Response {
  const path = new URL(request.url).pathname;
  if (path === "/v1/me") return jsonResponse(meFixture(SHARER));
  if (RECORD_READ.test(path)) {
    return jsonResponse({ id: path.split("/").pop(), writable: true });
  }
  if (path === "/v1/records/references") {
    return jsonResponse({
      data: [{ id: "d-1", name: "Acme renewal", archived: false }],
    });
  }
  if (request.url.includes("/users")) return jsonResponse(usersPage);
  if (request.url.includes("/teams")) return jsonResponse(teamsPage);
  if (request.url.includes("/record-grants")) {
    return jsonResponse({
      data: [existingGrant],
      page: { next_cursor: null, has_more: false },
    });
  }
  return jsonResponse({}, 404);
}

// The access answer of a contact owned by `ownerId`.
export function ownedAccess(ownerId: string): Response {
  return jsonResponse({
    visibility: "owner",
    owner_id: ownerId,
    archived: false,
    detail: false,
    you: {
      can_change: true,
      read_reasons: [{ code: "owner" }],
      change_reasons: [{ code: "owner" }],
    },
    data: [],
    page: { has_more: false, total: 0 },
    group_counts: { owner: 0, shared: 0, team_shared: 0, everyone: 0 },
    can_change_count: 0,
    team_access_count: 0,
    refresh_at: null,
  });
}
