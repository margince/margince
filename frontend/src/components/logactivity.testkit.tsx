import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { RecordZoneProvider } from "../lib/recordzone";
import { meFixture } from "../testing/mefixture";

// The composer's harness: a stubbed API that records each request, and a
// render inside the providers the form reads.

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The installation's zone this suite asserts against. Named rather than taken
// from the fallback: what these tests prove is that a logged entry's stored
// instant is cut on the INSTALLATION's clock, and a test that minted and read
// back on the same default would agree with itself whichever zone the code
// actually used.
export const INSTALLATION_ZONE = "Asia/Ho_Chi_Minh";

export function render(ui: ReactNode, zone = INSTALLATION_ZONE) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordZoneProvider zone={zone}>{ui}</RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function sentBody(
  request: Request | null,
  init: RequestInit | undefined,
): Promise<unknown> {
  try {
    return request ? await request.json() : JSON.parse(String(init?.body));
  } catch {
    return null;
  }
}

// The composer asks whether this seat may log at all before it draws the
// form, so every spec holds the grant the form requires. A spec that needs to
// withhold it names its own /me.
function defaultAnswer(key: string, url: URL): Response {
  if (key === "GET /me") {
    return jsonResponse(
      meFixture({ allow: { activity: ["create"], contact: ["read"] } }),
    );
  }
  if (url.pathname.endsWith("/context")) {
    return jsonResponse({
      anchor: { type: "contact", id: "p1" },
      sections: [],
    });
  }
  return jsonResponse(emptyPage);
}

export function stubApi(
  routes: Record<string, (body: unknown) => Response>,
  captured?: Captured[],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      const body = method === "GET" ? null : await sentBody(request, init);
      captured?.push({ key, body });
      const handler = routes[key];
      return handler ? handler(body) : defaultAnswer(key, url);
    }),
  );
}

export const contact = {
  id: "p1",
  full_name: "Petra Muster",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-07-06T08:00:00Z",
  updated_at: "2026-07-06T08:00:00Z",
};

export const createdActivity = (body: unknown) =>
  jsonResponse(
    {
      id: "a-new",
      kind: (body as { kind: string }).kind,
      subject: (body as { subject: string }).subject,
      occurred_at: "2026-07-06T09:00:00Z",
      captured_by: "human:u1",
      source: "manual",
      version: 1,
      created_at: "2026-07-06T09:00:00Z",
      updated_at: "2026-07-06T09:00:00Z",
    },
    201,
  );
