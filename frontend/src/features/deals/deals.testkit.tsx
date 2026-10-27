import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { RecordShell } from "../../testing/recordshell.testkit";

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export const render = (ui: ReactNode): RenderResult => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The region is the shell's in the running app (`main.tsx`); a suite whose
          subject is what a write says mounts it the same way. */}
        <ToastProvider>
          <RecordShell>{ui}</RecordShell>
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
};

export type Stage = components["schemas"]["Stage"];

export type Deal = components["schemas"]["Deal"];

export type Offer = components["schemas"]["Offer"];

export const stages: Stage[] = [
  {
    id: "s1",
    pipeline_id: "pl",
    name: "Qualify",
    position: 1,
    semantic: "open",
    win_probability: 20,
  },
  {
    id: "s2",
    pipeline_id: "pl",
    name: "Proposal",
    position: 2,
    semantic: "open",
    win_probability: 40,
  },
  {
    id: "s3",
    pipeline_id: "pl",
    name: "Won",
    position: 3,
    semantic: "won",
    win_probability: 100,
  },
];

export function deal(overrides: Partial<Deal>): Deal {
  return {
    id: "d1",
    name: "Fleet retrofit",
    amount_minor: 4_800_000,
    currency: "EUR",
    pipeline_id: "pl",
    stage_id: "s1",
    status: "open",
    source: "manual",
    captured_by: "human:u1",
    // The caller owns this deal, so the server sends writable: true. Absent
    // means not writable per the contract. A fixture that omits it describes a
    // reader every write control is withheld from.
    writable: true,
    version: 4,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  } as Deal;
}

// The grants a rep working their own pipeline holds, spelled once for every
// /me this file serves. They cover the deal verbs, the offer the panel starts,
// the stakeholder edges, and the activity the composer logs.
export const REP_GRANTS = {
  deal: ["read", "create", "update", "delete"],
  offer: ["read", "create"],
  relationship: ["read", "create", "update", "delete"],
  activity: ["read", "create"],
} as const;

export function offer(overrides: Partial<Offer>): Offer {
  return {
    id: "o1",
    deal_id: "d1",
    offer_number: "OFF-0001",
    revision: 1,
    status: "draft",
    currency: "EUR",
    net_minor: 100_000,
    tax_minor: 19_000,
    gross_minor: 119_000,
    ai_generated: false,
    line_items: [],
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  } as Offer;
}

export function stubBackend(
  deals: Deal[],
  opts: {
    onAdvance?: (body: unknown, ifMatch: string | null) => void;
    onCloseDay?: (body: unknown, ifMatch: string | null) => void;
    // Makes the stub enforce the server's win-evidence rule. A win naming
    // neither a contract nor a reason is refused 422, as the real server does.
    // Off by default, so every existing test keeps the deal that wins on the
    // first confirm.
    //
    // `true` refuses every deal. A list of ids refuses only those, so a test
    // can say "this deal has a contract and that one does not".
    demandsWinEvidence?: boolean | readonly string[];
    // What the win dialog's contract read answers. Left out, it tells the
    // truth about `demandsWinEvidence`, so the question comes before Confirm.
    // "signed" says there is a contract whatever the server later demands,
    // which is the contract archived while the dialog sat open. "unreadable"
    // fails the read.
    winEvidenceRead?: "signed" | "unreadable";
    single?: Deal;
    onPatch?: (body: unknown, ifMatch: string | null) => void;
    onDelete?: () => void;
    onBulk?: (path: string, body: unknown) => void;
    onDealsUrl?: (url: string) => void;
    pipelines?: components["schemas"]["Pipeline"][];
    agentTools?: components["schemas"]["AgentTool"][];
    stageTotalsRows?: Record<string, unknown>[];
    stageTotalsAsOf?: string;
    stageTotalsTimezone?: string;
    // Answers the totals report with this status and a problem body instead.
    stageTotalsStatus?: number;
    onStageTotalsBody?: (body: unknown) => void;
    savedViews?: Record<string, unknown>[];
    onCreateView?: (body: unknown) => void;
    // The second keyset page, served when the request carries a cursor.
    nextPage?: Deal[];
  } = {},
) {
  const demandsEvidence = (dealId: string) =>
    Array.isArray(opts.demandsWinEvidence)
      ? opts.demandsWinEvidence.includes(dealId)
      : Boolean(opts.demandsWinEvidence);
  // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a stub of every read and write the deal screens make; its branches are the route table
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    if (method === "GET" && url.includes("/win-evidence")) {
      if (opts.winEvidenceRead === "unreadable") {
        return jsonResponse({ code: "internal" }, 500);
      }
      const dealId = url.split("/deals/")[1]?.split("/")[0] ?? "";
      return jsonResponse({
        has_signed_contract:
          opts.winEvidenceRead === "signed" || !demandsEvidence(dealId),
      });
    }
    if (method === "GET" && url.includes("/users")) {
      return jsonResponse({
        data: [
          {
            id: "u-me",
            email: "me@acme.test",
            display_name: "Me",
            timezone: "UTC",
            status: "active",
            is_agent: false,
          },
        ],
        page: { next_cursor: null },
      });
    }
    if (method === "POST" && url.includes("/views")) {
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      opts.onCreateView?.(body);
      return jsonResponse({ id: "new-view", ...body }, 201);
    }
    if (method === "GET" && url.includes("/views")) {
      return jsonResponse({
        data: opts.savedViews ?? [],
        page: { next_cursor: null },
      });
    }
    // The record's tags. Without this the catch-all answers a shape with no
    // `withheld` key, which the panel reads as visible-and-empty: a state
    // asserted by accident rather than chosen.
    if (method === "GET" && url.includes("/tags")) {
      return jsonResponse({
        data: [
          {
            tag_id: "t-1",
            name: "Renewal",
            color: "teal",
            archived: false,
            assigned_at: "2026-03-03T10:00:00Z",
          },
        ],
        withheld: false,
      });
    }
    if (url.includes("/agent-tools")) {
      return jsonResponse({
        data: opts.agentTools ?? [],
        page: { next_cursor: null },
      });
    }
    if (url.includes("/context")) {
      return jsonResponse({ anchor: { type: "deal", id: "x" }, sections: [] });
    }
    if (url.includes("/pipelines")) {
      return jsonResponse({
        data: opts.pipelines ?? [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages,
          },
        ],
        page: { next_cursor: null },
      });
    }
    if (method === "POST" && url.includes("/reports/deals-by-stage")) {
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      opts.onStageTotalsBody?.(body);
      if (opts.stageTotalsStatus) {
        return jsonResponse(
          { status: opts.stageTotalsStatus, title: "Forbidden" },
          opts.stageTotalsStatus,
        );
      }
      return jsonResponse({
        report: "deals-by-stage",
        base_currency: "EUR",
        as_of: opts.stageTotalsAsOf ?? "2026-10-01",
        timezone: opts.stageTotalsTimezone,
        plan: {},
        columns: [],
        rows: opts.stageTotalsRows ?? [],
      });
    }
    if (method === "POST" && url.includes("/bulk/")) {
      const body = request ? await request.json() : {};
      opts.onBulk?.(new URL(url).pathname, body);
      return jsonResponse(
        url.includes("/preview")
          ? { ...body, count: 1, affected: [], excluded: [], sample: [] }
          : { batch_id: "b-1", changed: 1, skipped: [] },
      );
    }
    if (method === "POST" && url.includes("/close-day")) {
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      opts.onCloseDay?.(body, request?.headers.get("If-Match") ?? null);
      return jsonResponse(opts.single ?? deals[0]);
    }
    if (method === "POST" && url.includes("/advance")) {
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      opts.onAdvance?.(body, request?.headers.get("If-Match") ?? null);
      const advancedId = url.split("/deals/")[1]?.split("/")[0] ?? "";
      if (
        demandsEvidence(advancedId) &&
        body.status === "won" &&
        !body.won_without_contract_reason
      ) {
        return jsonResponse(
          {
            code: "validation_error",
            details: {
              errors: [
                {
                  field: "won_without_contract_reason",
                  code: "win_evidence_required",
                  message:
                    "a won deal needs a signed contract with its paper attached, or a reason why there is none",
                },
              ],
            },
          },
          422,
        );
      }
      return jsonResponse(deal({ stage_id: body.to_stage_id }));
    }
    if (method === "GET" && /\/deals\/[^/?]+(\?.*)?$/.test(url)) {
      return jsonResponse(opts.single ?? deals[0]);
    }
    if (method === "PATCH") {
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      const ifMatch = request ? request.headers.get("If-Match") : null;
      opts.onPatch?.(body, ifMatch);
      return jsonResponse(opts.single ?? deals[0]);
    }
    if (method === "DELETE") {
      opts.onDelete?.();
      return jsonResponse(opts.single ?? deals[0]);
    }
    if (url.includes("/me")) {
      return jsonResponse({
        user: {
          id: "u-me",
          email: "me@acme.test",
          display_name: "Me",
          timezone: "UTC",
          status: "active",
          is_agent: false,
        },
        roles: ["admin"],
        teams: [],
        // A rep reads companies: the deal screens name and search them.
        authorization: meFixture({
          allow: { ...REP_GRANTS, company: ["read"] },
        }).authorization,
      });
    }
    if (url.includes("/companies")) {
      return jsonResponse({
        data: [{ id: "o1", display_name: "Acme" }],
        page: { next_cursor: null },
      });
    }
    if (url.includes("/deals") && !url.includes("/outcome-reviews")) {
      opts.onDealsUrl?.(url);
      if (opts.nextPage) {
        return url.includes("cursor=")
          ? jsonResponse({
              data: opts.nextPage,
              page: { next_cursor: null, has_more: false },
            })
          : jsonResponse({
              data: deals,
              page: { next_cursor: "cur-2", has_more: true },
            });
      }
      return jsonResponse({ data: deals, page: { next_cursor: null } });
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}
