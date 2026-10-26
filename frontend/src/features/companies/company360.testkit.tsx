import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { RecordShell } from "../../testing/recordshell.testkit";
import { CompanyScreen } from "./companies";
import { ProposedNextSteps, type SuggestionAction } from "./company360";
import { type CompanyOverviewProps, CompanyReading } from "./companyreading";
import { CompanyTasksFinds } from "./companytasksfinds";

// The rules the composite read exists to keep:
//
//   - a section the caller's role withheld says so, and never draws the
//     empty state that would read as "there is none";
//   - consent is per purpose and default-deny, so silence never renders as
//     permission.

export type Company = components["schemas"]["Company"];

export type Company360 = components["schemas"]["Company360"];

// Typed against the contract, not asserted into it. An untyped fixture can name
// a dropped field, a retired enum value, or miss a required field. The tests on
// it keep passing while the shape moves. So `view()` returns `Company360`, and
// every caller takes it as that type rather than through `as never`.
export const company: Company = {
  id: "o-1",
  // The server answers this per row. Without it the fixture reads as not
  // writable (the fail-closed default) and loses the edit controls under test.
  writable: true,
  display_name: "Brandt Automotive GmbH",
  industry: "automotive",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

export const emptyPage = { has_more: false, next_cursor: null };

export function view(overrides: Partial<Company360> = {}): Company360 {
  return {
    as_of: "2026-06-01T09:00:00Z",
    company,
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
    ...overrides,
  };
}

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

export const emptyRollup = {
  root_id: "o-1",
  scope: "tree",
  weighted_pipeline: { amount_minor: 0, currency: "EUR" },
  closed_won: { amount_minor: 0, currency: "EUR" },
  activity_count_30d: 0,
  aggregated_account_count: 1,
  restricted_excluded: [],
  computed_at: "2026-06-01T09:00:00Z",
};

export const EMPTY_BRIEF = {
  company_id: "o-1",
  generated_at: "2026-06-01T09:00:00Z",
  generated_by: "deterministic",
  sentences: [],
};

// The brief every suite is served. No case sets its own, so it is a constant.
export const briefBody: unknown = EMPTY_BRIEF;

// partnerCompany has a partner programme, so it carries the second tab. The
// relationship type marks it, since the Company read never selects the extension
// row; the store keeps the two in step. The bare fixture carries neither,
// because the tab gate hides a Partner tab on an account with no programme.
export const partnerCompany = { ...company, relationship_types: ["partner"] };

export function stub(
  three60: unknown,
  status = 200,
  account: unknown = company,
  finance: unknown = { company_id: "o-1", state: "no_connection" },
  financeStatus = 200,
) {
  // The paths the page requested. A test proves the page did not refetch by
  // counting these.
  const fetched: string[] = [];
  vi.stubGlobal(
    "fetch",
    // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a stub of every read the company view makes; its branches are the route table
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      fetched.push(pathname);
      if (pathname.endsWith("/360")) {
        return jsonResponse(three60, status);
      }
      // The Contacts tab's list comes from the same contacts section the 360
      // fixture carries. A seeded contact shows on the tab without a second copy.
      if (pathname.endsWith("/contacts")) {
        const contacts =
          (three60 as { contacts?: { data?: Record<string, unknown>[] } })
            ?.contacts?.data ?? [];
        return jsonResponse({
          data: contacts.map((contact) => ({
            contact_id: contact.contact_id,
            full_name: contact.full_name,
            title: contact.title,
            engagement: "untried",
            strength: contact.strength,
          })),
          page: { has_more: false, next_cursor: null },
        });
      }
      if (pathname.endsWith("/finance-summary")) {
        return jsonResponse(finance, financeStatus);
      }
      if (pathname.endsWith("/hierarchy-rollup")) {
        return jsonResponse(emptyRollup);
      }
      if (pathname.endsWith("/brief")) {
        return jsonResponse(briefBody);
      }
      if (pathname.endsWith("/graph")) {
        return jsonResponse({
          nodes: [
            { id: "u-2", kind: "user", label: "Mira", root: false },
            { id: "p-1", kind: "contact", label: "Dana Buyer", root: false },
          ],
          edges: [
            {
              from: "u-2",
              to: "p-1",
              kind: "in_contact_with",
              strength: 90,
              strength_bucket: "strong",
            },
          ],
          groups_omitted: [],
          dropped_count: 0,
        });
      }
      // The viewer's grants. Without them useCan fails closed and every in-place
      // editor renders as text, so "withheld" would look the same as "never built".
      if (pathname.endsWith("/v1/me")) {
        return jsonResponse(
          meFixture({ allow: { company: ["read", "update"] } }),
        );
      }
      if (pathname.endsWith("/companies/o-1")) {
        return jsonResponse(account);
      }
      if (pathname.endsWith("/pipelines")) {
        // One default pipeline with one open stage, so a deal created from
        // this page has somewhere to land.
        return jsonResponse({
          data: [
            {
              id: "pl-1",
              name: "Sales",
              is_default: true,
              stages: [
                {
                  id: "st-1",
                  pipeline_id: "pl-1",
                  name: "Qualify",
                  position: 1,
                  semantic: "open",
                  probability: 10,
                },
              ],
            },
          ],
          page: emptyPage,
        });
      }
      return jsonResponse({ data: [], page: emptyPage });
    }),
  );
  return fetched;
}

// The context column belongs to the shell, and the record fills it through a
// portal. So this render carries the real region. A stand-in column would prove
// nothing about the product's one, and the context cards would have nowhere to go.
export function render(ui: ReactNode): RenderResult {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

export function renderCompany() {
  render(<CompanyScreen id="o-1" />);
}

// The Overview's head as the record page mounts it, every verb a no-op unless
// the test hands its own in.
export function overviewReading(
  three60: Company360,
  verbs: Partial<CompanyOverviewProps> = {},
): ReactNode {
  return (
    <CompanyReading
      company={three60.company}
      view={three60}
      loading={false}
      failed={false}
      onOpenRecord={() => {}}
      onOpenEmail={() => {}}
      onOpenTasks={() => {}}
      onOpenTask={() => {}}
      onLogActivity={() => {}}
      onDraftTo={() => {}}
      onOpenTab={() => {}}
      {...verbs}
    />
  );
}

// The advice rows, through the Tasks tab the record page mounts them on: the
// proposed steps, then the other finds.
export function renderSuggestions(
  three60: Company360,
  onPerform: (action: SuggestionAction) => void = () => {},
) {
  render(
    <>
      <ProposedNextSteps
        companyId={three60.company.id}
        view={three60}
        onOpenReceipt={() => {}}
      />
      <CompanyTasksFinds
        companyId={three60.company.id}
        view={three60}
        onPrepareMeeting={() => {}}
        onDraftTo={() => {}}
        onOpenRecord={() => {}}
        onOpenReceipt={() => {}}
        onOpenEmail={() => {}}
        onPerform={onPerform}
      />
    </>,
  );
}
