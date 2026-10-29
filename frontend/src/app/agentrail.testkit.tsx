import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  type RenderResult,
  render as rtlRender,
  waitFor,
} from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import type { ReactNode, RefObject } from "react";
import { expect, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import type { Route } from "../lib/router";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { AgentRail } from "./agentrail";

// The agent section at the foot of the workspace rail draws what the API said.
// It never shows a number nobody computed, or a read that has not answered.
//
// Red is a tool that cannot be reached: no model bound, or a source it cannot
// get to. Amber is a run that stalled or settled degraded. A transient tool
// failure colours nothing.

export type Connector = components["schemas"]["CaptureConnection"];

export type AssistantProfile = components["schemas"]["AssistantProfile"];

export type LicenseEntitlement = components["schemas"]["LicenseEntitlement"];

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export const emptyPage = { has_more: false, next_cursor: null };

export const CONNECTED: Connector = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  can_backfill: true,
  status: "connected",
  scopes: ["https://www.googleapis.com/auth/gmail.readonly"],
};

export const OPERATOR: GrantSpec = {
  ai_diagnostics: ["read"],
  license: ["read"],
};

export const PROFILE = (
  state: AssistantProfile["state"],
): AssistantProfile => ({
  name: "Margince",
  kind: "ai",
  state,
  inference_mode:
    state === "development"
      ? "development"
      : state === "unconfigured"
        ? "none"
        : "cloud",
  providers: [],
});

export const LICENSE = (
  state: LicenseEntitlement["state"],
): LicenseEntitlement => ({
  state,
  seats_used: 1,
  over_limit: false,
  checked_at: "2026-08-01T09:00:00Z",
});

/** A month with no priced line: the summary carries no figure. */
export const UNPRICED_SPEND = {
  currency: "USD",
  computed_at: "2026-08-01T09:00:00Z",
};

export const COMPANY_360_VIEW = {
  as_of: "2026-08-01T09:00:00Z",
  company: {
    id: "o-1",
    display_name: "Brandt Automotive GmbH",
    captured_by: "human:u1",
    source: "manual",
    version: 1,
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
  },
  sections_omitted: [],
  contacts: { data: [], page: emptyPage },
  deals: {
    data: [],
    page: emptyPage,
    won_lifetime: { amount_minor: 0, currency: "EUR" },
    lost_count: 0,
  },
  activities: { data: [], page: emptyPage },
  next_steps: { data: [], page: emptyPage },
  pending_approvals: { data: [], page: emptyPage },
  tags: [],
  since_last_visit: {
    baseline_at: "2026-05-30T09:00:00Z",
    new_activities: 0,
    deal_stage_moves: 0,
    pending_proposals: 0,
  },
};

export type FetchRoutes = {
  connectors?: () => Response | Promise<Response>;
  dedupe?: () => Response | Promise<Response>;
  approvals?: () => Response | Promise<Response>;
  aiCalls?: () => Response | Promise<Response>;
  aiSpend?: () => Response | Promise<Response>;
  me?: () => Response | Promise<Response>;
  profile?: () => Response | Promise<Response>;
  license?: () => Response | Promise<Response>;
  agentActivity?: () => Response | Promise<Response>;
};

// Routes every hook the section depends on by pathname, the way
// company360.test.tsx does for its own composite read. The healthy default
// answers every source as connected and every queue empty. The seat may read
// the runtime and the licence, the AI posture is configured and the licence
// is valid. Each test overrides only the one route its case is about. Returns
// the stubbed fetch mock, so a case can prove a request was never made (the
// 360 read off a company page).
export function stubAgentRailApi(routes: FetchRoutes = {}) {
  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a stub of every read the rail makes; its branches are the route table
  const fetchMock = vi.fn(async (request: Request) => {
    const pathname = new URL(request.url).pathname;
    if (pathname.endsWith("/connectors")) {
      return routes.connectors
        ? routes.connectors()
        : jsonResponse({ data: [CONNECTED] });
    }
    if (pathname.endsWith("/dedupe/candidates")) {
      return routes.dedupe
        ? routes.dedupe()
        : jsonResponse({ data: [], page: emptyPage });
    }
    if (pathname.endsWith("/approvals")) {
      return routes.approvals
        ? routes.approvals()
        : jsonResponse({ data: [], page: emptyPage });
    }
    if (pathname.endsWith("/ai/calls")) {
      return routes.aiCalls
        ? routes.aiCalls()
        : jsonResponse({
            data: [],
            page: emptyPage,
            payload_capture_enabled: false,
            ...{ tasks: [], task_options: [] },
          });
    }
    if (pathname.endsWith("/ai/usage/summary")) {
      return routes.aiSpend ? routes.aiSpend() : jsonResponse(UNPRICED_SPEND);
    }
    if (pathname.endsWith("/me/ai-activity")) {
      return routes.agentActivity
        ? routes.agentActivity()
        : jsonResponse({ running: [], recent: [], faults: [] });
    }
    if (pathname.endsWith("/me")) {
      return routes.me
        ? routes.me()
        : jsonResponse(meFixture({ allow: OPERATOR }));
    }
    if (pathname.endsWith("/assistant/profile")) {
      return routes.profile
        ? routes.profile()
        : jsonResponse(PROFILE("configured"));
    }
    if (pathname.endsWith("/installation/license")) {
      return routes.license ? routes.license() : jsonResponse(LICENSE("valid"));
    }
    if (pathname.endsWith("/360")) {
      return jsonResponse({ state: "ready", view: COMPANY_360_VIEW });
    }
    return jsonResponse({ data: [], page: emptyPage });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

export function render(
  route: Route,
  options: Readonly<{
    client?: QueryClient;
    children?: ReactNode;
    bar?: RefObject<HTMLElement | null>;
  }> = {},
): RenderResult {
  const client =
    options.client ??
    new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {options.children}
        <AgentRail route={route} bar={options.bar} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

export const ROUTE: Route = { screen: "deals" };

export const block = (container: HTMLElement) => {
  const el = container.querySelector(".arblock");
  if (!el) throw new Error("no .arblock in the rendered tree");
  return el;
};

export const openPanel = async (user: UserEvent, container: HTMLElement) => {
  const trigger = container.querySelector(".arhit");
  if (!trigger) throw new Error("no .arhit trigger in the rendered tree");
  await user.click(trigger);
};

// The panel is portalled to document.body once open, so its content is never
// found inside the render container.
export const panel = () => {
  const el = document.querySelector(".arloose");
  if (!el) throw new Error("no .arloose panel on the document");
  return el;
};

/**
 * The state's own resting line, once the ticker has finished naming the
 * mount-time reads.
 *
 * Every query the section holds starts a `useAgentTicker` line the moment it
 * fetches (agentrail-ticker.ts). That line outlives its own request by
 * `LINGER_MS` (1400ms) so a fast read is still readable. A case that asserts
 * the state's own line has to wait past that window. Otherwise it asserts
 * about the ticker's last word instead.
 */
// Named so it does not begin with `waitFor`. scripts/test-budget.ts treats
// that prefix as a library waiter. It then cannot fold the budget inside a
// local helper that wears it.
export async function settlesOnLine(container: HTMLElement, expected: string) {
  await waitFor(
    () =>
      expect(container.querySelector(".arline")?.textContent).toBe(expected),
    { timeout: 3000 },
  );
}
