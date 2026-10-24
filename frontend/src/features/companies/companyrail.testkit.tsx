import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ComponentProps, ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { CompanyOverviewCards } from "./companyoverviewcards";
import { CompanyRail } from "./companyrail";
import type { CompanyTab } from "./companytab";

export type Company360 = components["schemas"]["Company360"];

// The details' and the Overview cards' display rules. A section the caller's
// role withheld says so, instead of an empty state that reads as "there is
// none". A field the record does not carry folds behind "Show N empty fields".

export const company = {
  // Absent reads as not writable, which is the fail-closed default a real
  // response never relies on: the server answers this per row.
  writable: true,
  id: "o-1",
  workspace_id: "w",
  display_name: "Brandt Automotive GmbH",
  legal_name: "Brandt Automotive GmbH",
  lifecycle: "customer" as const,
  owner_id: "u-1",
  industry: "automotive" as const,
  size_band: "51-200" as const,
  linkedin_url: "https://linkedin.com/company/brandt",
  address: { city: "Munich", country: "DE" },
  domains: [
    {
      id: "dom-1",
      domain: "brandt.example",
      is_primary: true,
      source: "manual",
      captured_by: "human:u1",
    },
  ],
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

export const emptyPage = { has_more: false, next_cursor: null };

// The 360 this suite reads: every section present and empty, so a case names
// only the one it is about. Typed as the contract declares it, never cast.
export function view(overrides: Record<string, unknown> = {}): Company360 {
  return {
    as_of: "2026-06-01T09:00:00Z",
    company,
    sections_omitted: [],
    contacts: { data: [], page: emptyPage },
    deals: {
      data: [],
      page: emptyPage,
      won_lifetime: { amount_minor: 0, currency: null },
      lost_count: 0,
    },
    projects: [],
    projects_page: emptyPage,
    tags: [],
    ...overrides,
  };
}

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
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

export type RailProps = ComponentProps<typeof CompanyRail>;

// Every site below wants the same rail: a writable view, not loading.
// `overrides` supplies whatever the test varies.
export function renderRail(overrides: Partial<RailProps> = {}): RenderResult {
  const drawn = overrides.view ?? view();
  return render(
    <CompanyRail
      company={drawn.company}
      view={drawn}
      loading={false}
      {...overrides}
    />,
  );
}

// The Overview's four cards, read off the same 360.
export function renderCards({
  view: drawn = view(),
  onTab = () => {},
}: Readonly<{
  view?: Company360;
  onTab?: (tab: CompanyTab) => void;
}> = {}): RenderResult {
  return render(
    <CompanyOverviewCards
      view={drawn}
      onOpenTab={onTab}
      onOpenTask={() => {}}
      onOpenEmail={() => {}}
    />,
  );
}

// The paths every test needs, whatever it asserts. Health reads the finance
// summary for its payment dimension, the owner row resolves against the
// roster, and the rail reads the signals feed. `overrides` answers the rest.
export function stub(
  overrides: Record<
    string,
    (req: Request) => Response | Promise<Response>
  > = {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      for (const [suffix, respond] of Object.entries(overrides)) {
        if (pathname.endsWith(suffix)) {
          return await respond(request);
        }
      }
      if (pathname.endsWith("/finance-summary")) {
        return jsonResponse({ company_id: "o-1", state: "no_connection" });
      }
      if (pathname.endsWith("/users")) {
        return jsonResponse({
          data: [{ id: "u-1", display_name: "Mira Voss" }],
          page: emptyPage,
        });
      }
      // A real seat, spelled out. The catch-all below answers `/me` with an
      // empty page, which `useCan` reads as a principal holding no grants.
      // Then every gated control stands down and the suite proves nothing.
      if (pathname.endsWith("/me")) {
        return jsonResponse(
          meFixture({ allow: { company: ["read", "update"] } }),
        );
      }
      if (pathname.endsWith("/signals")) {
        return jsonResponse({ data: [], page: emptyPage });
      }
      return jsonResponse({ data: [], page: emptyPage });
    }),
  );
}
