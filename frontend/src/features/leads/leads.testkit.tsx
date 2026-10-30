import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  type RenderResult,
  render as rtlRender,
  screen,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { RecordShell } from "../../testing/recordshell.testkit";
import { WriteToHost } from "../settings/writetohost";
import { SIGNAL_CATALOG } from "./leadsignals.fixture";

// Edit, Share and Disqualify live behind the header's overflow trigger, whose
// panel mounts on the first open. So a test wanting one opens the menu first.
export async function openLeadActions(
  user: Pick<UserEvent, "click"> = userEvent,
) {
  await user.click(await screen.findByRole("button", { name: "More actions" }));
}

// The seven shipped lead origins GET /lead-origins serves on a fresh install.
export const SHIPPED_LEAD_ORIGINS = {
  data: [
    ["event", "Event", "neutral"],
    ["linkedin", "LinkedIn", "neutral"],
    ["referral", "Referral", "high"],
    ["partner", "Partner", "high"],
    ["website", "Website", "high"],
    ["cold_outreach", "Cold outreach", "low"],
    ["other", "Other", "neutral"],
  ].map(([key, label, intent], i) => ({
    id: `origin-${key}`,
    key,
    label,
    intent,
    sort_order: (i + 1) * 10,
    active: true,
    system: true,
    lead_count: 0,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  })),
};

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function render(
  ui: ReactNode,
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  }),
): RenderResult {
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The region is the shell's in the running app (`main.tsx`); a suite whose
          subject is what a write says mounts it the same way. */}
        <ToastProvider>
          {/* The composer host is the shell's too (`App.tsx`): the header's
              address is a button into it. */}
          <WriteToHost>
            <RecordShell>{ui}</RecordShell>
          </WriteToHost>
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The grants a rep working their own leads holds, spelled once for every
// /me this file serves.
export const LEAD_GRANTS = {
  lead: ["read", "create", "update", "delete"],
  activity: ["read", "create"],
} as const;

export const lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted" as const,
  score: 72,
  captured_by: "human:u-1",
  source: "manual",
  // The caller's own lead: absent means not writable per the contract, and
  // a fixture that omits it describes a reader every write here refuses.
  writable: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

// A URL-capturing fetch stub shared by the wiring tests. Every request is
// recorded so a test can assert the params it carried, and a caller-supplied
// responder decides what comes back.
export function stubFetch(
  responder: (
    url: string,
    method: string,
    request: Request,
  ) => Promise<Response>,
): { fetchMock: ReturnType<typeof vi.fn>; urls: string[] } {
  const urls: string[] = [];
  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a stub of every read and write the lead screens make; its branches are the route table
  const fetchMock = vi.fn(async (request: Request) => {
    urls.push(request.url);
    if (new URL(request.url).pathname.endsWith("/context")) {
      return jsonResponse({
        anchor: { type: "lead", id: "l-1" },
        sections: [],
      });
    }
    // The reader's mail connections, which the shell reads for every page:
    // none here, so the lead's address is the reader's own mail client's.
    if (request.method === "GET" && request.url.endsWith("/v1/connectors")) {
      return jsonResponse({ data: [] });
    }
    // What the lead sent and nobody answered: nothing, unless a test says so.
    if (new URL(request.url).pathname.endsWith("/activities/latest-inbound")) {
      return jsonResponse({ data: [] });
    }
    if (new URL(request.url).pathname.endsWith("/lead-signal-catalog")) {
      return jsonResponse(SIGNAL_CATALOG);
    }
    // The administered vocabularies, as a fresh installation ships them:
    // the screens read their labels and pick lists off these.
    if (request.method === "GET" && request.url.endsWith("/v1/lead-origins")) {
      return jsonResponse(SHIPPED_LEAD_ORIGINS);
    }
    if (
      request.method === "GET" &&
      request.url.endsWith("/v1/leads/settings")
    ) {
      return jsonResponse({
        first_response_enabled: true,
        first_response_target_minutes: 240,
      });
    }
    const answer = await responder(request.url, request.method, request);
    if (request.url.endsWith("/v1/me")) {
      const body: unknown = await answer.clone().json();
      if (typeof body === "object" && body !== null && "user" in body) {
        return answer;
      }
      return jsonResponse({
        user: { id: "u-9", display_name: "Me" },
        roles: ["rep"],
        teams: [],
        authorization: meFixture({ allow: LEAD_GRANTS }).authorization,
      });
    }
    if (request.method === "GET" && request.url.includes("/manual-signals")) {
      const body: unknown = await answer.clone().json();
      if (
        typeof body !== "object" ||
        body === null ||
        !("data" in body) ||
        !Array.isArray(body.data)
      ) {
        return jsonResponse({ data: [] });
      }
    }
    return answer;
  });
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, urls };
}

export function emptyPage() {
  return jsonResponse({
    data: [],
    page: { next_cursor: null, has_more: false },
  });
}

// A URL-capturing fetch stub that also answers the /v1/me probe. The status,
// score override and assign-to-me controls all need the session principal
// resolved. So every test using it sets a workspace slug and serves the probe
// alongside the lead responses.
export function stubFetchWithMe(
  responder: (
    url: string,
    method: string,
    request: Request,
  ) => Promise<Response | undefined>,
  meId = "u-9",
  // The reader's own mail connections, which the shell reads for every page.
  // They decide whether the lead's address is the composer's or their mail
  // client's. None, unless a suite about writing to the lead says so.
  connectors: unknown = { data: [] },
  // The settings surfaces this installation offers, for a suite about a card
  // that draws only where one is on (the lists card).
  settingsAvailability: Partial<
    components["schemas"]["SettingsAvailability"]
  > = {},
): { urls: string[] } {
  const urls: string[] = [];
  vi.stubGlobal(
    "fetch",
    // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a stub of every read and write the lead screens make; its branches are the route table
    vi.fn(async (request: Request) => {
      urls.push(request.url);
      if (request.method === "GET" && request.url.endsWith("/v1/connectors")) {
        return jsonResponse(connectors);
      }
      if (request.url.endsWith("/v1/me")) {
        const me = meFixture({ allow: LEAD_GRANTS, settingsAvailability });
        return jsonResponse({
          user: { id: meId, display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: me.authorization,
          settings_availability: me.settings_availability,
        });
      }
      if (new URL(request.url).pathname.endsWith("/context")) {
        return jsonResponse({
          anchor: { type: "lead", id: "l-1" },
          sections: [],
        });
      }
      if (
        new URL(request.url).pathname.endsWith("/activities/latest-inbound")
      ) {
        return jsonResponse({ data: [] });
      }
      if (new URL(request.url).pathname.endsWith("/lead-signal-catalog")) {
        return jsonResponse(SIGNAL_CATALOG);
      }
      if (
        request.method === "GET" &&
        request.url.endsWith("/v1/lead-origins")
      ) {
        return jsonResponse(SHIPPED_LEAD_ORIGINS);
      }
      if (
        request.method === "GET" &&
        request.url.endsWith("/v1/leads/settings")
      ) {
        return jsonResponse({
          first_response_enabled: true,
          first_response_target_minutes: 240,
        });
      }
      const answer = await responder(request.url, request.method, request);
      if (
        request.method === "GET" &&
        request.url.includes("/manual-signals") &&
        answer
      ) {
        const body: unknown = await answer.clone().json();
        if (
          typeof body !== "object" ||
          body === null ||
          !("data" in body) ||
          !Array.isArray(body.data)
        ) {
          return jsonResponse({ data: [] });
        }
      }
      return answer ?? jsonResponse(lead);
    }),
  );
  return { urls };
}
