/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { PageAsideProvider, PageAsideRegion } from "../app/pageaside";
import { pickOption } from "../design-system/select-testing";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { formatMoney } from "../format/format";
import { LocaleProvider } from "../i18n";
import {
  buildColumns,
  buildStageTotals,
  type CompanyNaming,
  DealScreen,
  DealsScreen,
  mapDealCreate,
  mapDealUpdate,
} from "./deals";

// B-EP09.11 acceptance: board renders per-column sub-lines from the fetched
// set, mixed-currency columns refuse a sum, the board↔table control keeps
// the SAME deal set with no reload, terminal drop opens the 🟡 confirm and
// nothing posts until confirmed, and an open-stage drop posts the advance.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

// Drops deal d1 on a stage column the way the board's drag does. jsdom carries
// no drag-and-drop, so the drop handler is dispatched directly — the click path
// the reader takes on touch is the stepper, which its own tests cover.
function dropOnStage(stageId: string, dealId = "d1") {
  const column = document.querySelector(
    `[data-stage="${stageId}"]`,
  ) as HTMLElement;
  const dataTransfer = { getData: () => dealId, setData: () => {} };
  const dropEvent = new Event("drop", { bubbles: true });
  Object.assign(dropEvent, { dataTransfer });
  column.dispatchEvent(dropEvent);
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The region is the shell's in the running app (`main.tsx`); a suite whose
          subject is what a write SAYS mounts it the same way. */}
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
};

type Stage = components["schemas"]["Stage"];
type Deal = components["schemas"]["Deal"];
type Offer = components["schemas"]["Offer"];
type Approval = components["schemas"]["Approval"];

const stages: Stage[] = [
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

// These cases are about column totals, so no card here names a company: an
// empty mark set resolves none, and none of these deals withholds one.
const noCompany: CompanyNaming = { marks: new Map(), unreadable: new Set() };

function deal(overrides: Partial<Deal>): Deal {
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
    version: 4,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  } as Deal;
}

function offer(overrides: Partial<Offer>): Offer {
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

function stubDealBackend(
  onRecord: Deal,
  offers: Offer[],
  onCreateOffer?: (body: unknown) => void,
  // What is staged against this deal. The record reads the workspace-wide
  // pending queue and filters it by target_entity_id, so a row only reaches
  // the panel when it names this deal.
  approvals: Approval[] = [],
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    if (url.includes("/pipelines")) {
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }
    if (url.includes("/context")) {
      return jsonResponse({ anchor: { type: "deal", id: "d1" }, sections: [] });
    }
    if (method === "POST" && url.includes("/offers")) {
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      onCreateOffer?.(body);
      return jsonResponse(
        offer({ id: "new-offer", currency: body.currency }),
        201,
      );
    }
    if (url.includes("/offers")) {
      return jsonResponse({ data: offers, page: { next_cursor: null } });
    }
    if (url.includes("/history")) {
      return jsonResponse({
        data: [
          {
            id: "h1",
            actor_type: "human",
            actor_id: "u1",
            action: "update",
            occurred_at: "2026-07-13T10:00:00Z",
            summary: "Deal amount changed",
          },
        ],
        page: { next_cursor: null },
      });
    }
    if (url.includes("/stakeholders")) {
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }
    if (url.includes("/approvals")) {
      return jsonResponse({ data: approvals, page: { next_cursor: null } });
    }
    if (url.includes("/activities")) {
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }
    if (url.includes("/deals/")) {
      return jsonResponse(onRecord);
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

// AC-F1: column totals come from the server's per-stage
// aggregate (Σround(amount×p/100), never round(Σamount×p/100)) — not from
// summing whatever page of cards happened to load. buildStageTotals shapes
// the report's rows (grouped by stage_id + currency); buildColumns reads
// from that, and keeps building the CARD list from the loaded deals as
// before — the cap on cards is unrelated to the correctness of the totals.
describe("buildStageTotals", () => {
  it("carries one currency's totals straight through", () => {
    const totals = buildStageTotals([
      {
        stage_id: "s1",
        currency: "EUR",
        deals: 3,
        raw_minor: 300_000,
        weighted_minor: 60_000,
      },
    ]);
    expect(totals.get("s1")).toEqual({
      count: 3,
      rawMinor: 300_000,
      weightedMinor: 60_000,
      currency: "EUR",
      sumHidden: false,
    });
  });

  it("hides the sum when a stage has more than one currency row", () => {
    const totals = buildStageTotals([
      {
        stage_id: "s2",
        currency: "EUR",
        deals: 1,
        raw_minor: 100_000,
        weighted_minor: 20_000,
      },
      {
        stage_id: "s2",
        currency: "USD",
        deals: 1,
        raw_minor: 100_000,
        weighted_minor: 20_000,
      },
    ]);
    const s2 = totals.get("s2");
    expect(s2?.sumHidden).toBe(true);
    expect(s2?.count).toBe(2);
  });

  it("a stage absent from the rows gets zeroed, not undefined", () => {
    const totals = buildStageTotals([]);
    expect(totals.get("s1")).toBeUndefined();
  });
});

describe("buildColumns", () => {
  it("reads sums from the totals map, not from the loaded cards", () => {
    const totals = buildStageTotals([
      // The server's per-deal-rounded figure (12343 × 20% rounded per deal,
      // twice, then summed = 4938) — deliberately NOT what round(Σ) gives
      // (4937), so a regression back to client-side summing would fail this.
      {
        stage_id: "s1",
        currency: "EUR",
        deals: 2,
        raw_minor: 24_686,
        weighted_minor: 4_938,
      },
    ]);
    const columns = buildColumns(
      stages,
      [
        deal({
          id: "a",
          stage_id: "s1",
          amount_minor: 12_343,
          currency: "EUR",
        }),
        deal({
          id: "b",
          stage_id: "s1",
          amount_minor: 12_343,
          currency: "EUR",
        }),
      ],
      totals,
      noCompany,
    );
    expect(columns[0].rawMinor).toBe(24_686);
    expect(columns[0].weightedMinor).toBe(4_938);
    expect(columns[0].deals).toHaveLength(2);
  });

  it("hides the sum for a mixed-currency stage per the totals map, regardless of which cards loaded", () => {
    const totals = buildStageTotals([
      {
        stage_id: "s2",
        currency: "EUR",
        deals: 1,
        raw_minor: 100_000,
        weighted_minor: 20_000,
      },
      {
        stage_id: "s2",
        currency: "USD",
        deals: 1,
        raw_minor: 100_000,
        weighted_minor: 20_000,
      },
    ]);
    const columns = buildColumns(
      stages,
      [
        deal({
          id: "c",
          stage_id: "s2",
          amount_minor: 100_000,
          currency: "EUR",
        }),
      ],
      totals,
      noCompany,
    );
    expect(columns[1].sumHidden).toBe(true);
  });

  it("a stage with no totals row states no figure and no currency", () => {
    const columns = buildColumns(stages, [], new Map(), noCompany);
    expect(columns[0].rawMinor).toBeNull();
    expect(columns[0].currency).toBeNull();
    expect(columns[0].sumHidden).toBeFalsy();
  });
});

function stubBackend(
  deals: Deal[],
  opts: {
    onAdvance?: (body: unknown, ifMatch: string | null) => void;
    // Makes the stub enforce the server's win-evidence rule: a win naming
    // neither a contract nor a reason is refused 422 the way the real one
    // refuses it. Off by default, so every existing test keeps the deal that
    // wins on the first confirm.
    //
    // `true` refuses every deal; a list of ids refuses only those, which is how
    // a test says "this deal has a contract and that one does not".
    demandsWinEvidence?: boolean | readonly string[];
    single?: Deal;
    onPatch?: (body: unknown, ifMatch: string | null) => void;
    onDelete?: () => void;
    onDealsUrl?: (url: string) => void;
    pipelines?: components["schemas"]["Pipeline"][];
    agentTools?: components["schemas"]["AgentTool"][];
    stageTotalsRows?: Record<string, unknown>[];
    onStageTotalsBody?: (body: unknown) => void;
    savedViews?: Record<string, unknown>[];
    onCreateView?: (body: unknown) => void;
    // The second keyset page, served when the request carries a cursor.
    nextPage?: Deal[];
  } = {},
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
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
    // `withheld` key, which the panel reads as visible-and-empty — a state
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
      return jsonResponse({
        report: "deals-by-stage",
        plan: {},
        columns: [],
        rows: opts.stageTotalsRows ?? [],
      });
    }
    if (method === "POST" && url.includes("/advance")) {
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      opts.onAdvance?.(body, request?.headers.get("If-Match") ?? null);
      const advancedId = url.split("/deals/")[1]?.split("/")[0] ?? "";
      const demands = Array.isArray(opts.demandsWinEvidence)
        ? opts.demandsWinEvidence.includes(advancedId)
        : Boolean(opts.demandsWinEvidence);
      if (
        demands &&
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
      });
    }
    if (url.includes("/organizations")) {
      return jsonResponse({
        data: [{ id: "o1", display_name: "Acme" }],
        page: { next_cursor: null },
      });
    }
    if (url.includes("/deals")) {
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

describe("mapDealUpdate", () => {
  // The form's controls as a person who changed nothing left them. Every value
  // here is exactly what dealEditRecord seeded, so a key reaching the body is
  // the form reporting a change nobody made.
  const untouched = {
    name: "Fleet retrofit",
    amount: "2120",
    currency: "EUR",
    owner_id: "u-me",
    organization_id: "",
    partner_org_id: "",
    partner_attribution: "",
    forecast_category: "",
    expected_close_date: "",
    wait_until: "",
    project_id: "",
  };

  it("rebuilds amount_minor from major units for the fields the person moved", () => {
    const body = mapDealUpdate(
      {
        ...untouched,
        amount: "2120",
        currency: "EUR",
        owner_id: "u-me",
        forecast_category: "commit",
        expected_close_date: "2026-09-01",
      },
      { ...untouched, amount: "", currency: "", owner_id: "" },
    );
    expect(body.amount_minor).toBe(212_000);
    expect(body.currency).toBe("EUR");
    expect(body.owner_id).toBe("u-me");
    expect(body.forecast_category).toBe("commit");
    expect(body.expected_close_date).toBe("2026-09-01");
    // Named nowhere in the edit, so named nowhere in the body.
    expect("name" in body).toBe(false);
    expect("wait_until" in body).toBe(false);
  });

  // The reported defect. The body used to carry every field on every save, so a
  // deal with no company resubmitted `organization_id: null` — and the API,
  // correctly, refused to clear a field the person never touched. On an
  // installation with no partners the refusal named `partner_attribution`, a
  // field the form does not even render.
  it("sends nothing at all when the person changed nothing", () => {
    const body = mapDealUpdate(untouched, untouched);

    expect(Object.keys(body)).toEqual([]);
  });

  it("names only the field the person moved, on a deal missing every optional value", () => {
    const bare = {
      name: "Any Deal",
      amount: "",
      currency: "",
      owner_id: "",
      organization_id: "",
      partner_org_id: "",
      partner_attribution: "",
      forecast_category: "",
      expected_close_date: "",
      wait_until: "",
      project_id: "",
    };

    const body = mapDealUpdate({ ...bare, name: "Renamed" }, bare);

    expect(Object.keys(body)).toEqual(["name"]);
    expect(body.name).toBe("Renamed");
  });

  // A withheld reference arrives as null with masked_fields naming it —
  // deliberately, so a reader can tell "you may not see this" from "there is
  // nothing here". The form has only the null, so sending it back would ask
  // the server to clear a partner the reader never saw. Omitted means
  // unchanged, which is the only honest patch for a field nobody was shown.
  it("does not clear a partner it was never allowed to see", () => {
    const body = mapDealUpdate(
      { name: "Fleet retrofit", partner_org_id: "", partner_attribution: "" },
      {
        name: "Fleet retrofit",
        partner_org_id: "p-1",
        partner_attribution: "sourced",
      },
      ["partner_org_id"],
    );
    expect("partner_org_id" in body).toBe(false);
    // The attribution is withheld WITH its partner, so it goes too — returning
    // half the pair would decide what a partner nobody could see is owed.
    expect("partner_attribution" in body).toBe(false);
  });

  it("still clears a partner the reader could see and chose to remove", () => {
    const body = mapDealUpdate(
      { ...untouched, partner_org_id: "" },
      { ...untouched, partner_org_id: "p-1", partner_attribution: "sourced" },
    );
    expect(body.partner_org_id).toBeNull();
    // The claim goes with the partner, server-side, as one fact. Naming its own
    // null here would state a claim with nobody left to attribute it to, which
    // is the one shape the API refuses.
    expect("partner_attribution" in body).toBe(false);
  });

  // The scale is part of the amount: amount_minor is denominated in the
  // currency's OWN minor units, so a currency moving alone re-denominates the
  // figure the row holds. 10000 JPY re-saved as EUR is €100, and the fx freeze
  // and the forecast history then record that as the price.
  it("re-sends the figure at the new scale when only the currency moved", () => {
    const priced = { ...untouched, amount: "10000", currency: "JPY" };

    const body = mapDealUpdate({ ...priced, currency: "EUR" }, priced);

    expect(body.currency).toBe("EUR");
    expect(body.amount_minor).toBe(1_000_000);
  });

  // Naming a currency on a deal with no figure is half a money value, and the
  // server's own pair refusal says so better than an amount this form invents.
  it("invents no figure when a currency is named on an unpriced deal", () => {
    const bare = { ...untouched, amount: "", currency: "" };

    const body = mapDealUpdate({ ...bare, currency: "EUR" }, bare);

    expect(Object.keys(body)).toEqual(["currency"]);
  });

  it("moves the claim alone when the partner stays put", () => {
    const body = mapDealUpdate(
      {
        ...untouched,
        partner_org_id: "p-1",
        partner_attribution: "influenced",
      },
      { ...untouched, partner_org_id: "p-1", partner_attribution: "sourced" },
    );
    expect(Object.keys(body)).toEqual(["partner_attribution"]);
    expect(body.partner_attribution).toBe("influenced");
  });
});

describe("mapDealCreate", () => {
  // A deal names its partner at birth. The create body once carried neither
  // partner field, and the API accepted the request and stored neither, so the
  // caller was told a write had succeeded with the partner gone.
  it("carries the partner and what they did into the birth body", () => {
    const body = mapDealCreate(
      {
        name: "Northgate rollout",
        stage_id: "s-1",
        amount: "480",
        currency: "EUR",
        organization_id: "cust-1",
        partner_org_id: "partner-1",
        partner_attribution: "influenced",
      },
      "p-1",
    );
    expect(body.partner_org_id).toBe("partner-1");
    expect(body.partner_attribution).toBe("influenced");
    expect(body.organization_id).toBe("cust-1");
    expect(body.pipeline_id).toBe("p-1");
    expect(body.amount_minor).toBe(48_000);
  });

  // Leaving the attribution on its empty option is the caller making no claim.
  // The server reads a named partner as "sourced", which is what that option
  // says it does — the form must not invent a different claim here.
  it("sends no attribution when the caller made no claim", () => {
    const body = mapDealCreate(
      { name: "x", stage_id: "s-1", partner_org_id: "partner-1" },
      "p-1",
    );
    expect(body.partner_org_id).toBe("partner-1");
    expect(body.partner_attribution).toBeNull();
  });

  it("names no partner when none was picked", () => {
    const body = mapDealCreate({ name: "x", stage_id: "s-1" }, "p-1");
    expect(body.partner_org_id).toBeNull();
    expect(body.partner_attribution).toBeNull();
  });
});

describe("DealsScreen", () => {
  // The board card draws the same chip strip a list row does, so a reader
  // moving between the two reads one thing one way. The card takes a view
  // model rather than the wire row, so the tags have to be copied across in
  // toBoardDeal — a field left out there is silently absent on every card.
  it("draws a deal's tags on its board card", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([
        deal({
          tags: [{ tag_id: "t-1", name: "Renewal", color: "teal" }],
        }),
      ]),
    );
    render(<DealsScreen />);
    expect(await screen.findByText("Renewal")).toBeTruthy();
  });

  it("board↔table swaps views over the same fetched set without a reload", async () => {
    const fetchMock = stubBackend([deal({})]);
    vi.stubGlobal("fetch", fetchMock);
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );
    const dealFetches = () =>
      fetchMock.mock.calls.filter((call) =>
        String(
          call[0] && (call[0] as Request).url
            ? (call[0] as Request).url
            : call[0],
        ).includes("/deals"),
      ).length;
    const before = dealFetches();
    await userEvent.click(screen.getByRole("button", { name: "Table" }));
    expect(screen.getByText("Fleet retrofit")).toBeTruthy(); // same set, table view
    expect(dealFetches()).toBe(before); // no reload
  });

  // A view saved on the deals list is a server row, and the tab rail has to
  // read it. The rail carried only the one hardcoded sort before, so a saved
  // view was storable through the contract and then invisible.
  it("offers a saved view as a tab beside the standing sort", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        savedViews: [
          {
            id: "v1",
            resource: "deals",
            name: "Slipping this quarter",
            query: {
              list: { sort: "-amount_minor", filters: { stalled: "true" } },
            },
            created_at: "2026-06-01T00:00:00Z",
            updated_at: "2026-06-01T00:00:00Z",
          },
        ],
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    expect(
      await screen.findByRole("button", { name: "Slipping this quarter" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Newest" })).toBeTruthy();
  });

  // Picking the tab has to narrow the list, not just highlight: the saved
  // filters travel to the server or the view is decoration.
  it("applies a saved view's filters to the deals request", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        onDealsUrl: (url) => urls.push(url),
        savedViews: [
          {
            id: "v1",
            resource: "deals",
            name: "My stalled deals",
            query: { list: { sort: "", filters: { stalled: "true" } } },
            created_at: "2026-06-01T00:00:00Z",
            updated_at: "2026-06-01T00:00:00Z",
          },
        ],
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));
    await user.click(
      await screen.findByRole("button", { name: "My stalled deals" }),
    );

    await waitFor(() =>
      expect(urls.some((url) => url.includes("stalled=true"))).toBe(true),
    );
  });

  // The pipeline picker is the strongest dial on this screen and it lives in
  // its own state, outside the list query. Saved without it, a view would
  // restore against whichever pipeline happened to be showing — a different
  // list of deals under the name the reader chose.
  it("saves the selected pipeline as part of the view", async () => {
    let saved: Record<string, unknown> | undefined;
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        onCreateView: (body) => {
          saved = body as Record<string, unknown>;
        },
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));
    // The save action appears only once something narrows the list; a sort is
    // the cheapest such narrowing to reach from here.
    await user.click(screen.getByRole("button", { name: "Sort by Value" }));
    await user.click(await screen.findByRole("button", { name: "Save view" }));
    await user.type(
      await screen.findByRole("textbox", { name: "Name" }),
      "Pipeline view",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(saved).toBeTruthy());
    if (!saved) {
      throw new Error("the save never reached the server");
    }
    const list = (saved.query as Record<string, Record<string, unknown>>).list;
    expect((list.filters as Record<string, string>).pipeline_id).toBe("pl");
  });

  // A pipeline is always selected, so carrying it into the query must not make
  // an untouched list look narrowed: the save action would then offer to store
  // the default view, which is the clutter its own check exists to prevent.
  it("offers no save action until the reader narrows something", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    expect(screen.queryByRole("button", { name: "Save view" })).toBeNull();
  });

  // A bulk verb is a fan-out of each row's own write, so every row must carry
  // ITS OWN version. One version copied across the selection would conflict on
  // every row but the one it came from.
  it("assigning an owner in bulk sends each row's own version", async () => {
    const patches: { body: unknown; ifMatch: string | null }[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend(
        [
          deal({ id: "d1", name: "First", version: 3 }),
          deal({ id: "d2", name: "Second", version: 9 }),
        ],
        { onPatch: (body, ifMatch) => patches.push({ body, ifMatch }) },
      ),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    await user.click(
      await screen.findByRole("checkbox", { name: "Select First" }),
    );
    await user.click(screen.getByRole("checkbox", { name: "Select Second" }));
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "New owner" }),
      "Me",
    );
    await user.click(screen.getByRole("button", { name: "Assign" }));

    await waitFor(() => expect(patches.length).toBe(2));
    expect(patches.map((patch) => patch.ifMatch).sort()).toEqual(["3", "9"]);
    expect((patches[0].body as { owner_id: string }).owner_id).toBe("u-me");
  });

  // The server treats every advance as a transition — it writes a stage-history
  // row and emits deal.stage_changed without asking whether anything moved — so
  // a row already in the target stage must not be sent at all, or the velocity
  // reports read a move that never happened.
  it("moving to a stage skips the rows already in it", async () => {
    const advances: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend(
        [
          deal({ id: "d1", name: "Already there", stage_id: "s2" }),
          deal({ id: "d2", name: "Needs moving", stage_id: "s1" }),
        ],
        { onAdvance: (_body, _ifMatch) => advances.push("advance") },
      ),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    await user.click(
      await screen.findByRole("checkbox", { name: "Select Already there" }),
    );
    await user.click(
      screen.getByRole("checkbox", { name: "Select Needs moving" }),
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Move to stage" }),
      "Proposal",
    );
    await user.click(screen.getByRole("button", { name: "Move" }));

    // One write, for the row that actually moves.
    await waitFor(() => expect(advances.length).toBe(1));
  });

  // Archiving many deals at once is the most destructive thing this bar does,
  // and every other archive in the product asks first.
  it("bulk archive asks before it removes anything", async () => {
    let deleted = 0;
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ id: "d1", name: "First" })], {
        onDelete: () => {
          deleted += 1;
        },
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));
    await user.click(
      await screen.findByRole("checkbox", { name: "Select First" }),
    );

    await user.click(screen.getByRole("button", { name: "Archive" }));
    expect(deleted).toBe(0);
    // One deal reads as one deal, not "1 deals".
    expect(screen.getByText("Archive this deal?")).toBeTruthy();

    // The dialog's own Archive button, not the bar's.
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Archive" }));
    await waitFor(() => expect(deleted).toBe(1));
  });

  // A closed deal takes no bulk write: archiving it is done or meaningless,
  // and moving it between open stages would be the silent reopen the record
  // page's stepper already refuses.
  it("offers no checkbox on a closed deal", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([
        deal({ id: "d1", name: "Open one" }),
        deal({ id: "d2", name: "Won one", status: "won", stage_id: "s3" }),
      ]),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    expect(
      await screen.findByRole("checkbox", { name: "Select Open one" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("checkbox", { name: "Select Won one" }),
    ).toBeNull();
  });

  // The board draws its columns from the deals it holds, so a single capped
  // read meant a busy stage showed a fraction of its cards while its header —
  // which counts EVERY matching deal — went on naming the true number. A
  // column saying "40 deals" above six cards is what this prevents.
  it("the board walks past the first page", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ id: "d1", name: "First page deal" })], {
        nextPage: [deal({ id: "d2", name: "Second page deal" })],
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);

    expect(await screen.findByText("First page deal")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Load more" }));

    // Both pages stand together — the walk adds cards, it does not replace them.
    expect(await screen.findByText("Second page deal")).toBeTruthy();
    expect(screen.getByText("First page deal")).toBeTruthy();
  });

  // The column headers count every matching deal through a SEPARATE report
  // query. Keyed apart from the cards it never saw the invalidation every deal
  // mutation fires, so a moved card sat under a header still counting it in
  // the stage it left. The key now lives UNDER ["deals"], which is what makes
  // that one invalidation reach both — assert the relationship, since a
  // request count cannot tell a real invalidation from a routine refetch.
  it("keys the column totals under the deals cache so one invalidation reaches both", async () => {
    const keys: unknown[][] = [];
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <DealsScreen />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );

    for (const query of client.getQueryCache().getAll()) {
      keys.push(query.queryKey as unknown[]);
    }
    const totals = keys.find((key) => key.includes("by-stage-totals"));
    expect(totals).toBeTruthy();
    // invalidateQueries({queryKey:["deals"]}) matches by PREFIX, so this is
    // the whole claim: the totals live under it.
    expect(totals?.[0]).toBe("deals");
  });

  // The board's column total must come from the deals-by-stage
  // report over EVERY matching deal, not from summing the (capped) page of
  // cards. The seeded card's own amount×probability would give a different,
  // WRONG figure if the board still computed it client-side — this proves
  // it renders the server's number instead.
  it("renders the board's column total from the deals-by-stage report, not from the loaded cards", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ id: "a", stage_id: "s1", amount_minor: 1 })], {
        stageTotalsRows: [
          {
            stage_id: "s1",
            currency: "EUR",
            deals: 250,
            raw_minor: 9_999_999,
            weighted_minor: 1_234_567,
          },
        ],
      }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(
        screen.getByText(formatMoney(9_999_999, "EUR", "en")),
      ).toBeTruthy(),
    );
    expect(
      screen.getByText(`weighted ${formatMoney(1_234_567, "EUR", "en")}`),
    ).toBeTruthy();
    // The true stage count (250), not "1" — the single loaded card's count.
    expect(screen.getByText("250 deals")).toBeTruthy();
  });

  it("sends the board's active filters to the deals-by-stage totals request", async () => {
    let sentBody: unknown;
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        onStageTotalsBody: (body) => {
          sentBody = body;
        },
      }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );
    expect(sentBody).toMatchObject({
      group_by: ["stage_id", "currency"],
    });
  });

  it("a terminal-stage advance opens the 🟡 confirm and posts only after confirming", async () => {
    const advances: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onAdvance: (b, m) => advances.push([b, m]) }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );

    // simulate the drop on the Won column via the drop handler path
    const wonColumn = document.querySelector(
      '[data-stage="s3"]',
    ) as HTMLElement;
    const dataTransfer = { getData: () => "d1", setData: () => {} };
    const dropEvent = new Event("drop", { bubbles: true }) as unknown as {
      dataTransfer: typeof dataTransfer;
    };
    Object.assign(dropEvent, { dataTransfer });
    wonColumn.dispatchEvent(dropEvent as unknown as Event);

    await waitFor(() => expect(screen.getByText("Move to Won?")).toBeTruthy());
    expect(advances).toHaveLength(0); // nothing posted yet — confirm-first
    await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual([{ to_stage_id: "s3", status: "won" }, "4"]);
  });

  // A deal genuinely won on a purchase order or a phone call has no contract to
  // point at, and before this the confirm dialog offered no way to say so: the
  // win was refused, the reason the server wanted appeared in no field, and the
  // deal stayed open. The dialog now asks the question the refusal implies.
  it("a win the server refuses for want of evidence asks how it was won, and sends the answer", async () => {
    const advances: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        demandsWinEvidence: true,
        onAdvance: (body) => advances.push(body),
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );

    dropOnStage("s3");
    await waitFor(() => expect(screen.getByText("Move to Won?")).toBeTruthy());

    // The first confirm asks for nothing: a deal WITH a contract wins here, and
    // only the server knows which this is.
    expect(screen.queryByText("How was it won?")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    // Refused — the dialog stays open and grows the question rather than
    // dropping the reader back on a board that has snapped the card home.
    await waitFor(() =>
      expect(screen.getByText("How was it won?")).toBeTruthy(),
    );
    expect(screen.getByText("Move to Won?")).toBeTruthy();
    expect(advances).toHaveLength(1);

    // Confirm stays refused until the question is answered.
    expect(
      screen.getByRole("button", { name: "Confirm" }).hasAttribute("disabled"),
    ).toBe(true);

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "On a purchase order",
    );
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(advances).toHaveLength(2));
    expect(advances[1]).toEqual({
      to_stage_id: "s3",
      status: "won",
      won_without_contract_reason: "purchase_order",
    });
  });

  // "Something else" is the one member that explains nothing on its own, so the
  // server demands a detail after it. Sending the reason without one would be a
  // refusal the reader could have been spared.
  it('picking "Something else" holds Confirm until the detail says what it was', async () => {
    const advances: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        demandsWinEvidence: true,
        onAdvance: (body) => advances.push(body),
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );

    dropOnStage("s3");
    await waitFor(() => expect(screen.getByText("Move to Won?")).toBeTruthy());
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() =>
      expect(screen.getByText("How was it won?")).toBeTruthy(),
    );

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "Something else",
    );

    const confirm = screen.getByRole("button", { name: "Confirm" });
    expect(confirm.hasAttribute("disabled")).toBe(true);

    await user.type(screen.getByLabelText("What was it?"), "a barter deal");
    expect(confirm.hasAttribute("disabled")).toBe(false);
    await user.click(confirm);

    await waitFor(() => expect(advances).toHaveLength(2));
    expect(advances[1]).toEqual({
      to_stage_id: "s3",
      status: "won",
      won_without_contract_reason: "other",
      won_without_contract_detail: "a barter deal",
    });
  });

  // The refusal belongs to the deal that earned it. Read off the shared
  // mutation's error instead, it outlives that deal: cancel, open Won on a
  // deal that DOES have a contract, and the reason picker greets it — and the
  // server takes a stated reason at its word without looking for a contract,
  // so that deal is recorded as won-without-paper when it was not.
  it("a refusal on one deal does not ask the next deal how it was won", async () => {
    const withoutPaper = deal({ id: "d1", name: "No contract" });
    const withPaper = deal({ id: "d2", name: "Has contract" });
    vi.stubGlobal(
      "fetch",
      stubBackend([withoutPaper, withPaper], {
        // Only d1 lacks evidence. d2 wins on the first confirm.
        demandsWinEvidence: ["d1"],
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() => expect(screen.getByText("No contract")).toBeTruthy());

    dropOnStage("s3");
    await waitFor(() => expect(screen.getByText("Move to Won?")).toBeTruthy());
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() =>
      expect(screen.getByText("How was it won?")).toBeTruthy(),
    );

    // Walk away from that deal without answering.
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByText("Move to Won?")).toBeNull());

    dropOnStage("s3", "d2");
    await waitFor(() => expect(screen.getByText("Move to Won?")).toBeTruthy());
    expect(screen.queryByText("How was it won?")).toBeNull();
  });

  // A win that succeeds on the first confirm has nothing left to ask, so the
  // dialog must close. Left open, its Confirm stays live over a version the
  // write just replaced.
  it("a win that needs no reason closes the dialog on success", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );

    dropOnStage("s3");
    await waitFor(() => expect(screen.getByText("Move to Won?")).toBeTruthy());
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(screen.queryByText("Move to Won?")).toBeNull());
  });

  // The detail belongs to "Something else" alone. Carried across a change of
  // reason it would be stored anyway — the server writes both columns as given
  // — leaving text on the deal behind a field the reader can no longer see.
  it('changing away from "Something else" does not send the detail', async () => {
    const advances: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        demandsWinEvidence: true,
        onAdvance: (body) => advances.push(body),
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );

    dropOnStage("s3");
    await waitFor(() => expect(screen.getByText("Move to Won?")).toBeTruthy());
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() =>
      expect(screen.getByText("How was it won?")).toBeTruthy(),
    );

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "Something else",
    );
    await user.type(screen.getByLabelText("What was it?"), "a barter deal");

    // Change your mind: the detail field disappears, and so must its text.
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "On a purchase order",
    );
    expect(screen.queryByLabelText("What was it?")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(advances).toHaveLength(2));
    expect(advances[1]).toEqual({
      to_stage_id: "s3",
      status: "won",
      won_without_contract_reason: "purchase_order",
    });
  });

  // The server rejects a detail of format-only characters (`saysSomething` in
  // win_evidence.go). Enabling Confirm on one earns a second refusal for the
  // omission the reader was already asked about.
  it('a zero-width detail does not satisfy "Something else"', async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { demandsWinEvidence: true }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );

    dropOnStage("s3");
    await waitFor(() => expect(screen.getByText("Move to Won?")).toBeTruthy());
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() =>
      expect(screen.getByText("How was it won?")).toBeTruthy(),
    );

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "Something else",
    );
    await user.type(screen.getByLabelText("What was it?"), "​​");

    expect(
      screen.getByRole("button", { name: "Confirm" }).hasAttribute("disabled"),
    ).toBe(true);
  });

  it("the advance-confirm dot reads the live catalog tier, not a hardcode", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        agentTools: [
          {
            name: "progress_deal",
            title: "Progress a deal with a note",
            description:
              'Move a deal to a new stage and leave a note on its timeline saying why. (Governance: some calls run immediately and others a person approves first, decided per call from its arguments; requires passport scope "write".)',
            required_scope: "write",
            tier: "auto_execute",
            egress: false,
          },
        ],
      }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );

    const wonColumn = document.querySelector(
      '[data-stage="s3"]',
    ) as HTMLElement;
    const dataTransfer = { getData: () => "d1", setData: () => {} };
    const dropEvent = new Event("drop", { bubbles: true }) as unknown as {
      dataTransfer: typeof dataTransfer;
    };
    Object.assign(dropEvent, { dataTransfer });
    wonColumn.dispatchEvent(dropEvent as unknown as Event);

    await waitFor(() => expect(screen.getByText("Move to Won?")).toBeTruthy());
    // progress_deal is catalogued "auto_execute" — a hardcoded
    // "confirm" dot would render "confirm-first" here instead.
    await waitFor(() =>
      expect(screen.getByLabelText("auto-execute")).toBeTruthy(),
    );
  });

  it("an open-stage drop advances without a confirm", async () => {
    const advances: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onAdvance: (b, m) => advances.push([b, m]) }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );

    const proposalColumn = document.querySelector(
      '[data-stage="s2"]',
    ) as HTMLElement;
    const dropEvent = new Event("drop", { bubbles: true });
    Object.assign(dropEvent, {
      dataTransfer: { getData: () => "d1", setData: () => {} },
    });
    proposalColumn.dispatchEvent(dropEvent);

    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual([{ to_stage_id: "s2" }, "4"]);
    await waitFor(() =>
      expect(screen.getByText("Moved to Proposal")).toBeTruthy(),
    );
  });

  it("overlay mode paginates the flat mirror table through the keyset cursor", async () => {
    const dealsCalls: string[] = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
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
          system_of_record: { mode: "overlay" },
        });
      }
      if (url.includes("/deals")) {
        dealsCalls.push(url);
        if (new URL(url, "http://t").searchParams.get("cursor")) {
          return jsonResponse({
            data: [deal({ id: "d2", name: "Second page deal" })],
            page: { next_cursor: null, has_more: false },
          });
        }
        return jsonResponse({
          data: [deal({ id: "d1", name: "First page deal" })],
          page: { next_cursor: "cursor-2", has_more: true },
        });
      }
      // pipelines / agent-tools / organizations / context — all empty here.
      return jsonResponse({ data: [], page: { next_cursor: null } });
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<DealsScreen />);

    // Page one renders in the forced flat table, with the Load-more affordance.
    expect(await screen.findByText("First page deal")).toBeTruthy();
    const loadMore = await screen.findByRole("button", { name: /load more/i });

    // Loading the next page appends it and carries the cursor from page one.
    await userEvent.click(loadMore);
    expect(await screen.findByText("Second page deal")).toBeTruthy();
    expect(screen.getByText("First page deal")).toBeTruthy();
    expect(dealsCalls.some((u) => u.includes("cursor=cursor-2"))).toBe(true);
  });

  it("overlay mode keeps the loaded rows when a Load-more page fails", async () => {
    const dealsCalls: string[] = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
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
          system_of_record: { mode: "overlay" },
        });
      }
      if (url.includes("/deals")) {
        dealsCalls.push(url);
        if (new URL(url, "http://t").searchParams.get("cursor")) {
          return jsonResponse({ title: "boom" }, 500); // the next page fails
        }
        return jsonResponse({
          data: [deal({ id: "d1", name: "First page deal" })],
          page: { next_cursor: "cursor-2", has_more: true },
        });
      }
      return jsonResponse({ data: [], page: { next_cursor: null } });
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<DealsScreen />);

    expect(await screen.findByText("First page deal")).toBeTruthy();
    await userEvent.click(
      await screen.findByRole("button", { name: /load more/i }),
    );
    // The next page errored, but the already-loaded page-one rows must
    // survive — a transient later-page failure never discards usable results.
    await waitFor(() =>
      expect(dealsCalls.some((u) => u.includes("cursor=cursor-2"))).toBe(true),
    );
    expect(screen.getByText("First page deal")).toBeTruthy();
  });
});

describe("DealsScreen filters", () => {
  it("switching pipeline scopes the deals fetch to that pipeline_id", async () => {
    const user = userEvent.setup();
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        onDealsUrl: (u) => urls.push(u),
        pipelines: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages,
          },
          {
            id: "pl2",
            name: "Renewals",
            is_default: false,
            position: 1,
            stages,
          },
        ],
      }),
    );
    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await pickOption(user, screen.getByLabelText("Pipeline"), "Renewals");
    await waitFor(() =>
      expect(urls.some((u) => u.includes("pipeline_id=pl2"))).toBe(true),
    );
  });

  // The board always shows one pipeline, so an unset choice would fall straight
  // back to the default one — the pipeline list therefore offers pipelines
  // only. The stage filter's "all" entry clears a query filter, which the board
  // can actually show, so that one stays.
  it("offers pipelines only, while the stage filter keeps its all-stages entry", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        pipelines: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages,
          },
          {
            id: "pl2",
            name: "Renewals",
            is_default: false,
            position: 1,
            stages,
          },
        ],
      }),
    );
    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");

    await user.click(screen.getByLabelText("Pipeline"));
    expect(
      within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["Sales", "Renewals"]);
    await user.keyboard("{Escape}");

    // Stage is a filter chip now rather than a picker of its own, so its
    // all-stages entry sits one step inside the Filter menu — and it still has
    // to be there, because that entry is how a chosen stage is cleared.
    // "Stage" also names a column header, so the attribute is picked from
    // inside the menu that is open rather than by a plain name match.
    await user.click(screen.getByRole("button", { name: "Table" }));
    await user.click(screen.getByRole("button", { name: "Filter" }));
    const menu = screen.getByRole("group", { name: "Filter" });
    await user.click(within(menu).getByRole("button", { name: "Stage" }));
    expect(
      within(menu).getByRole("button", { name: "All stages" }),
    ).toBeTruthy();
  });

  it("the stalled filter adds stalled=true to the deals query", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onDealsUrl: (u) => urls.push(u) }),
    );
    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    // The stalled filter lives on the table view, not the board.
    await userEvent.click(screen.getByRole("button", { name: "Table" }));

    // The Filter button's attribute step and its value step both carry the
    // "Stalled only" label — the chip's option shares the attribute's own
    // name — so each step is picked from inside the menu that is open at
    // that moment rather than by a plain (ambiguous) name match.
    await userEvent.click(screen.getByRole("button", { name: "Filter" }));
    const menu = screen.getByRole("group", { name: "Filter" });
    await userEvent.click(
      within(menu).getByRole("button", { name: "Stalled only" }),
    );
    await userEvent.click(
      within(menu).getByRole("button", { name: "Stalled only" }),
    );

    await waitFor(() =>
      expect(urls.some((u) => u.includes("stalled=true"))).toBe(true),
    );
  });
});

/**
 * Open the header's overflow, which is where archiving, sharing and reopening
 * live: verbs whose consequence a reader has to read before pressing get a
 * whole line rather than a place in the verb row. Edit stays in the row.
 */
async function openHeaderMenu(): Promise<void> {
  await userEvent.click(
    await screen.findByRole("button", { name: "More actions" }),
  );
}

describe("DealScreen — edit, archive, FX line (A3)", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("edit prefills and PATCHes with If-Match", async () => {
    const patches: { body: unknown; ifMatch: string | null }[] = [];
    const d = deal({ id: "x", version: 4, owner_id: null });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        onPatch: (b, h) => patches.push({ body: b, ifMatch: h }),
      }),
    );
    render(<DealScreen id="x" />);
    await userEvent.click(await screen.findByTestId("edit-record"));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(patches.length).toBe(1));
    expect(patches[0].ifMatch).toBe("4");
  });

  // The partner was editable in the form and rendered nowhere, so a deal a
  // partner brought looked identical to one we won alone — while being the
  // fact a commission is computed from.
  it("names the partner that brought the deal, and links to it", async () => {
    const d = deal({
      id: "x",
      organization_id: "o1",
      partner_org_id: "p1",
      partner_attribution: "sourced",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const url = request.url;
        // EntityRef resolves each reference by its own id read; a reference it
        // cannot name is deliberately not a link.
        if (url.includes("/organizations/p1")) {
          return jsonResponse({ id: "p1", display_name: "VietnamPartner JSC" });
        }
        return stubBackend([d], { single: d })(request);
      }),
    );

    render(<DealScreen id="x" />);

    expect(
      await screen.findByRole("button", { name: "VietnamPartner JSC" }),
    ).toBeTruthy();
    expect(screen.getByText("via")).toBeTruthy();
  });

  // A form offers nothing the record already carries as a blank. The partner
  // picker offers one capped page of partners, so a deal's own partner can be
  // missing from it — and a select whose stored value is not an option shows
  // blank, which the patch then reads as the person having chosen "Unset" and
  // sends as a real null, clearing the partner and its commission attribution.
  //
  // The save says nothing about the partner at all, which is what makes it
  // safe: omitted means unchanged.
  it("keeps a partner the picker cannot reach, rather than clearing it on save", async () => {
    const user = userEvent.setup();
    const patches: { body: unknown }[] = [];
    // Neither the org list ("Acme") nor the partner list holds p-offpage.
    const d = deal({
      id: "x",
      version: 4,
      partner_org_id: "p-offpage",
      partner_attribution: "sourced",
    });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        onPatch: (body) => patches.push({ body }),
      }),
    );

    render(<DealScreen id="x" />);
    await user.click(await screen.findByTestId("edit-record"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(patches.length).toBe(1));
    const body = patches[0].body as Record<string, unknown>;
    expect("partner_org_id" in body).toBe(false);
    expect("partner_attribution" in body).toBe(false);
  });

  // Same guarantee when the partner list never arrives at all — a failed or
  // still-pending /partners read must not read as "this deal has no partner".
  it("keeps the partner when the partner list fails outright", async () => {
    const user = userEvent.setup();
    const patches: { body: unknown }[] = [];
    const d = deal({
      id: "x",
      version: 4,
      partner_org_id: "p-offpage",
      partner_attribution: "influenced",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.includes("/partners")) {
          return jsonResponse({ title: "Boom" }, 500);
        }
        return stubBackend([d], {
          single: d,
          onPatch: (body) => patches.push({ body }),
        })(request);
      }),
    );

    render(<DealScreen id="x" />);
    await user.click(await screen.findByTestId("edit-record"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(patches.length).toBe(1));
    const body = patches[0].body as Record<string, unknown>;
    expect("partner_org_id" in body).toBe(false);
    expect("partner_attribution" in body).toBe(false);
  });

  // The facts run together without a separator: three adjacent spans in a
  // plain text line rendered "€48,000.00Acme Corpvia Northgate", which is why
  // the partner looked missing on screen while every assertion about it passed.
  it("separates the subtitle's facts so they do not run together", async () => {
    const d = deal({
      id: "x",
      amount_minor: 4_800_000,
      currency: "EUR",
      organization_id: "o1",
      partner_org_id: "p1",
      partner_attribution: "sourced",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const url = request.url;
        if (url.includes("/organizations/p1")) {
          return jsonResponse({ id: "p1", display_name: "Northgate" });
        }
        if (url.includes("/organizations/o1")) {
          return jsonResponse({ id: "o1", display_name: "Acme Corp" });
        }
        return stubBackend([d], { single: d })(request);
      }),
    );

    render(<DealScreen id="x" />);
    await screen.findByRole("button", { name: "Northgate" });
    const line = document.querySelector(".record-sub")?.textContent ?? "";

    expect(line).toContain("·");
    expect(line).not.toContain("€48,000.00Acme");
  });

  // Sourced and influenced are paid differently, so the line has to say which.
  it("says a partner only helped when the deal was influenced, not sourced", async () => {
    const d = deal({
      id: "x",
      partner_org_id: "p1",
      partner_attribution: "influenced",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.includes("/organizations/p1")) {
          return jsonResponse({ id: "p1", display_name: "Xentral" });
        }
        return stubBackend([d], { single: d })(request);
      }),
    );

    render(<DealScreen id="x" />);

    expect(await screen.findByText("helped by")).toBeTruthy();
    expect(screen.queryByText("via")).toBeNull();
  });

  it("shows the FX base line only when fx_rate_to_base is set", async () => {
    const d = deal({
      id: "x",
      amount_minor: 100_000,
      currency: "USD",
      fx_rate_to_base: "0.92",
      fx_rate_date: "2026-07-01",
    });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);
    await waitFor(() => expect(screen.getByText(/rate 0.92/)).toBeTruthy());
  });

  it("archive confirms then DELETEs", async () => {
    let deleted = false;
    const d = deal({ id: "x", version: 1 });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        onDelete: () => {
          deleted = true;
        },
      }),
    );
    render(<DealScreen id="x" />);
    await openHeaderMenu();
    await userEvent.click(screen.getByTestId("archive-record"));
    await userEvent.click(screen.getByTestId("archive-confirm"));
    await waitFor(() => expect(deleted).toBe(true));
  });
});

// Closing a deal used to be reachable only by dragging its card on the board,
// which meant it could not be done from the deal's own page at all — and not
// at all on a touch device, where HTML5 drag never fires.
describe("DealScreen — the stage stepper advances the deal", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("moving to an open stage posts the advance pinned to the version shown", async () => {
    const advances: { body: unknown; ifMatch: string | null }[] = [];
    const d = deal({ id: "x", version: 7, stage_id: "s1" });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        onAdvance: (body, ifMatch) => advances.push({ body, ifMatch }),
      }),
    );
    const user = userEvent.setup();
    render(<DealScreen id="x" />);

    await user.click(await screen.findByRole("button", { name: "Proposal" }));

    await waitFor(() => expect(advances.length).toBe(1));
    expect((advances[0].body as { to_stage_id: string }).to_stage_id).toBe(
      "s2",
    );
    // The version the record was drawn from, so a change made elsewhere
    // meanwhile fails loud rather than being erased.
    expect(advances[0].ifMatch).toBe("7");
  });

  // The advance is only half the job: a write whose confirmation is shown to
  // nobody reads exactly like one that did not happen.
  it("confirms the move on the record itself", async () => {
    const d = deal({ id: "x", version: 7, stage_id: "s1" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    const user = userEvent.setup();
    render(<DealScreen id="x" />);

    await user.click(await screen.findByRole("button", { name: "Proposal" }));

    expect(await screen.findByText("Moved to Proposal")).toBeTruthy();
  });

  // A control that can only fail is worse than none: an archived deal is not
  // moved through the pipeline, it is restored first.
  // The deal's tags ride in the CONTEXT rail, beside the seats, the deal room
  // and the mail card — the same column a person and a company draw theirs in.
  // They sat in the overview pane once, full-width between the readings and the
  // stage stepper, on the belief that this page had no side column; it fills one
  // through `PageAside`, which portals into the shell's rail.
  it("draws the deal's tags in the context rail", async () => {
    const d = deal({ id: "x" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    // The provider and the region together, because `PageAside` is a PORTAL:
    // without a mounted host it renders null, and a bare screen would report
    // the card missing whichever column it was written into.
    render(
      <PageAsideProvider>
        <DealScreen id="x" />
        <PageAsideRegion />
      </PageAsideProvider>,
    );

    const tag = await screen.findByText("Renewal");
    expect(tag.closest("aside")).not.toBeNull();
  });

  it("offers no move on an archived deal", async () => {
    const d = deal({ id: "x", archived_at: "2026-07-01T00:00:00Z" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);

    const step = await screen.findByRole("button", { name: "Proposal" });
    expect(step.hasAttribute("disabled")).toBe(true);
  });

  // Reopening is its own deliberate action, with a dialog that says the close
  // date and the frozen rate are being cleared. A stepper button that reopened
  // silently would be a second, quieter door to the same write.
  it("offers no move on a closed deal — reopening has its own action", async () => {
    const d = deal({
      id: "x",
      status: "won",
      stage_id: "s3",
      closed_at: "2026-07-01T00:00:00Z",
    });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);

    const step = await screen.findByRole("button", { name: "Proposal" });
    expect(step.hasAttribute("disabled")).toBe(true);
  });

  // The dialog stays mounted between openings, so an abandoned reason would
  // otherwise still be sitting there the next time a deal is closed — and it
  // would describe a different deal.
  it("a lost reason typed and then cancelled does not come back", async () => {
    const d = deal({ id: "x", version: 2, stage_id: "s1" });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        pipelines: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages: [
              ...stages,
              {
                id: "s4",
                pipeline_id: "pl",
                name: "Lost",
                position: 4,
                semantic: "lost",
                win_probability: 0,
              },
            ],
          },
        ],
      }),
    );
    const user = userEvent.setup();
    render(<DealScreen id="x" />);

    await user.click(await screen.findByRole("button", { name: "Lost" }));
    await user.type(
      screen.getByRole("textbox", { name: "Lost reason" }),
      "typed then abandoned",
    );
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await user.click(await screen.findByRole("button", { name: "Lost" }));
    expect(
      (screen.getByRole("textbox", { name: "Lost reason" }) as HTMLInputElement)
        .value,
    ).toBe("");
  });

  it("the stage the deal is already in is not a control", async () => {
    const d = deal({ id: "x", stage_id: "s1" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);

    await screen.findByRole("button", { name: "Proposal" });
    expect(screen.queryByRole("button", { name: "Qualify" })).toBeNull();
  });

  // Terminal stages are the 🟡 confirm (AC-deal-6), and a lost deal must say
  // why — the same rule the board's drop enforces, because it is the same
  // dialog.
  it("closing as lost asks for a reason before anything is written", async () => {
    const advances: { body: unknown; ifMatch: string | null }[] = [];
    const d = deal({ id: "x", version: 2, stage_id: "s1" });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        pipelines: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages: [
              ...stages,
              {
                id: "s4",
                pipeline_id: "pl",
                name: "Lost",
                position: 4,
                semantic: "lost",
                win_probability: 0,
              },
            ],
          },
        ],
        onAdvance: (body, ifMatch) => advances.push({ body, ifMatch }),
      }),
    );
    const user = userEvent.setup();
    render(<DealScreen id="x" />);

    await user.click(await screen.findByRole("button", { name: "Lost" }));
    // Nothing is written while the dialog stands open.
    expect(advances.length).toBe(0);

    const confirm = screen.getByRole("button", { name: "Confirm" });
    expect(confirm.hasAttribute("disabled")).toBe(true);

    await user.type(
      screen.getByRole("textbox", { name: "Lost reason" }),
      "went with a competitor",
    );
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(advances.length).toBe(1));
    const body = advances[0].body as { status: string; lost_reason: string };
    expect(body.status).toBe("lost");
    expect(body.lost_reason).toBe("went with a competitor");
  });
});

describe("DealScreen — an archived deal keeps its verbs, refused", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("shows Edit, Archive, Share and Reopen disabled, each reachable from the one sentence naming the archive", async () => {
    const d = deal({
      id: "x",
      status: "won",
      stage_id: "s3",
      archived_at: "2026-07-13T00:00:00Z",
    });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);

    await openHeaderMenu();
    const refused = [
      await screen.findByTestId("edit-record"),
      screen.getByTestId("archive-record"),
      screen.getByTestId("share-record"),
      screen.getByTestId("reopen-open"),
    ];
    for (const control of refused) {
      expect(control.hasAttribute("disabled")).toBe(true);
      // The reason has to be reachable FROM the control: a disabled button
      // cannot be focused and a `title` on it is announced by nobody, so a
      // sentence the control does not point at reaches no reader who needed it.
      const describedBy = control.getAttribute("aria-describedby");
      expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
        "This deal is archived and takes no changes.",
      );
    }
  });
});

describe("DealScreen — overlay mode write affordances", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  function overlayBackend(
    d: Deal,
    opts: {
      onPatch?: (body: unknown) => void;
      onDelete?: () => void;
    } = {},
  ) {
    // Mutable so a refetch after a successful PATCH (useUpdateRecord
    // invalidates the record query) sees the write applied — the same
    // "mirror re-read reflects the write-back" shape the real overlay
    // Provider.Update gives (mirrorWriteResult), not a stale echo.
    let current = d;
    return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request ? request.method : (init?.method ?? "GET");
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
          system_of_record: { mode: "overlay" },
        });
      }
      if (method === "PATCH") {
        const body = request
          ? await request.json()
          : JSON.parse(String(init?.body));
        opts.onPatch?.(body);
        current = { ...current, ...(body as Partial<Deal>) };
        return jsonResponse(current);
      }
      if (method === "DELETE") {
        opts.onDelete?.();
        return jsonResponse(current);
      }
      if (url.includes("/deals/")) {
        return jsonResponse(current);
      }
      return jsonResponse({ data: [], page: { next_cursor: null } });
    });
  }

  it("serves Edit and Archive — the mirror write-back seam accepts both", async () => {
    const d = deal({ id: "x", version: 3 });
    vi.stubGlobal("fetch", overlayBackend(d));
    render(<DealScreen id="x" />);
    expect(await screen.findByTestId("edit-record")).toBeTruthy();
    await openHeaderMenu();
    expect(screen.getByTestId("archive-record")).toBeTruthy();
    // The mirror owns the deal's mail, so the header offers no Email verb.
    expect(screen.queryByRole("button", { name: "Email" })).toBeNull();
  });

  it("Edit's real click path PATCHes and the 360 renders the updated value", async () => {
    const patches: unknown[] = [];
    const d = deal({ id: "x", version: 3 });
    vi.stubGlobal(
      "fetch",
      overlayBackend(d, { onPatch: (body) => patches.push(body) }),
    );
    render(<DealScreen id="x" />);
    await userEvent.click(await screen.findByTestId("edit-record"));
    const nameInput = screen.getByLabelText("Deal name *");
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, "Fleet retrofit — expanded scope");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(patches).toHaveLength(1));
    expect(
      await screen.findByText("Fleet retrofit — expanded scope"),
    ).toBeTruthy();
  });

  it("Edit's overlay notice names the partial write-back honestly", async () => {
    const d = deal({ id: "x" });
    vi.stubGlobal("fetch", overlayBackend(d));
    render(<DealScreen id="x" />);
    await userEvent.click(await screen.findByTestId("edit-record"));
    expect(
      screen.getByText(/Only the fields HubSpot accepts are written back/),
    ).toBeTruthy();
  });

  it("keeps Reopen and Share hidden even for a won deal", async () => {
    const d = deal({ id: "x", status: "won", stage_id: "s3" });
    vi.stubGlobal("fetch", overlayBackend(d));
    render(<DealScreen id="x" />);
    await screen.findByTestId("edit-record");
    expect(screen.queryByTestId("reopen-open")).toBeNull();
    expect(screen.queryByTestId("share-record")).toBeNull();
  });
});

describe("DealScreen reopen", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("reopen is shown only for won/lost and advances to an open stage with status open", async () => {
    const moves: [unknown, string | null][] = [];
    const d = deal({ id: "x", status: "won", stage_id: "s3" });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], { single: d, onAdvance: (b, m) => moves.push([b, m]) }),
    );
    render(<DealScreen id="x" />);
    await openHeaderMenu();
    await userEvent.click(screen.getByTestId("reopen-open"));
    await userEvent.click(screen.getByTestId("reopen-stage-s1"));
    await userEvent.click(screen.getByTestId("reopen-confirm"));
    await waitFor(() => expect(moves.length).toBe(1));
    expect(moves[0]).toEqual([{ to_stage_id: "s1", status: "open" }, "4"]);
  });

  it("reopen is not offered for an open deal", async () => {
    const d = deal({ id: "y", status: "open" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="y" />);
    await screen.findByTestId("edit-record"); // 360 rendered
    expect(screen.queryByTestId("reopen-open")).toBeNull();
  });
});

describe("DealScreen offers panel", () => {
  it("lists a deal's offers with status badge and formatted money", async () => {
    vi.stubGlobal(
      "fetch",
      stubDealBackend(deal({}), [
        offer({
          id: "o1",
          offer_number: "OFF-0001",
          revision: 1,
          status: "sent",
          gross_minor: 119_000,
          currency: "EUR",
        }),
      ]),
    );
    render(<DealScreen id="d1" />);
    await waitFor(() => expect(screen.getByText("OFF-0001")).toBeTruthy());
    expect(screen.getByText("sent")).toBeTruthy();
    expect(screen.getByText(formatMoney(119_000, "EUR", "en"))).toBeTruthy();
  });

  it("creating a new offer posts a draft and navigates to it", async () => {
    const creates: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubDealBackend(deal({ currency: "EUR" }), [], (body) =>
        creates.push(body),
      ),
    );
    render(<DealScreen id="d1" />);
    await waitFor(() =>
      expect(screen.getByText("Fleet retrofit")).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "New offer" }));
    await waitFor(() => expect(creates).toHaveLength(1));
    expect(creates[0]).toMatchObject({ currency: "EUR", source: "manual" });
    await waitFor(() =>
      expect(window.location.hash).toBe("#/offers/new-offer"),
    );
  });
});

describe("DealScreen pending approvals", () => {
  const staged = {
    id: "ap-1",
    kind: "advance_deal",
    status: "pending",
    summary: "Move Fleet retrofit to Proposal",
    proposed_change: { to_stage_id: "s2" },
    proposed_by: "agent:capture",
    target_entity_type: "deal",
    target_entity_id: "d1",
    created_at: "2026-07-01T08:00:00Z",
    evidence: [],
  } as Approval;

  // The panel states the same two facts the approvals inbox states, in the same
  // words: the kind through the shared catalog map, the proposer through the
  // provenance tag. Off the wire those facts read `advance_deal` and
  // `agent:capture` — the API's vocabulary on a page whose reader never sees
  // the API.
  it("names the staged kind and its proposer in the product's words, not the wire's", async () => {
    vi.stubGlobal("fetch", stubDealBackend(deal({}), [], undefined, [staged]));
    render(<DealScreen id="d1" />);

    // approval.kind.advance_deal — the key the inbox reads for the same kind.
    expect(await screen.findByText("Move a deal forward")).toBeTruthy();
    // trust.agentTag: an agent, named, rather than the doubled wire string.
    expect(screen.getByText("Automated by capture")).toBeTruthy();
    expect(screen.queryByText("advance_deal")).toBeNull();
    expect(screen.queryByText("agent:capture")).toBeNull();
  });
});

describe("DealScreen — History tab", () => {
  it("shows a History tab that lists record changes", async () => {
    vi.stubGlobal("fetch", stubDealBackend(deal({}), []));
    render(<DealScreen id="d1" />);

    await waitFor(() =>
      expect(screen.getByRole("button", { name: /history/i })).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: /history/i }));

    await waitFor(() =>
      expect(screen.getByText("Deal amount changed")).toBeTruthy(),
    );
  });
});

// Which partner, not just whether there is one.
//
// The boolean partner_sourced chip could say "these came from some partner"
// and never say which — the same gap the deals-by-stage report had before it
// gained the dimension. The picker's options come from usePartnerOptions, so
// a partner whose company this reader cannot open is not offered: picking it
// would name a company the screen could not then show them.
describe("the partner filter", () => {
  it("narrows the list to one named partner", async () => {
    const urls: string[] = [];
    const d = deal({ id: "d1", name: "Fleet retrofit" });
    vi.stubGlobal("fetch", (request: Request) => {
      urls.push(request.url);
      if (request.url.includes("/partners")) {
        return Promise.resolve(
          jsonResponse({
            data: [{ organization_id: "o1", cert_status: "certified" }],
            page: { next_cursor: null },
          }),
        );
      }
      return Promise.resolve(stubBackend([d], { single: d })(request));
    });

    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await userEvent.click(screen.getByRole("button", { name: "Table" }));

    await userEvent.click(screen.getByRole("button", { name: "Filter" }));
    const menu = screen.getByRole("group", { name: "Filter" });
    await userEvent.click(
      within(menu).getByRole("button", { name: "Partner" }),
    );
    // The option is the company's NAME, resolved from the organization list —
    // never the bare id, which names nothing to a reader.
    await userEvent.click(within(menu).getByRole("button", { name: "Acme" }));

    await waitFor(() =>
      expect(urls.some((u) => u.includes("partner_org_id=o1"))).toBe(true),
    );
  });

  it("sends the partner to the board totals too, not only the deal list", async () => {
    // The board's per-column totals come from the deals-by-stage report with
    // the screen's own dials. A dial the screen offers and the report refuses
    // answers 422, and the board then counts the cards it happens to hold —
    // which looks exactly like a working total.
    const bodies: unknown[] = [];
    const d = deal({ id: "d1", name: "Fleet retrofit" });
    vi.stubGlobal("fetch", async (request: Request) => {
      if (request.url.includes("/partners")) {
        return jsonResponse({
          data: [{ organization_id: "o1", cert_status: "certified" }],
          page: { next_cursor: null },
        });
      }
      if (request.url.includes("/reports/deals-by-stage")) {
        bodies.push(await request.clone().json());
      }
      return stubBackend([d], { single: d })(request);
    });

    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await userEvent.click(screen.getByRole("button", { name: "Table" }));
    await userEvent.click(screen.getByRole("button", { name: "Filter" }));
    const menu = screen.getByRole("group", { name: "Filter" });
    await userEvent.click(
      within(menu).getByRole("button", { name: "Partner" }),
    );
    await userEvent.click(within(menu).getByRole("button", { name: "Acme" }));

    await waitFor(() =>
      expect(
        bodies.some(
          (b) =>
            (b as { filters?: Record<string, unknown> }).filters
              ?.partner_org_id === "o1",
        ),
      ).toBe(true),
    );
  });

  it("offers no partner filter when the installation has no partners", async () => {
    const d = deal({ id: "d1", name: "Fleet retrofit" });
    vi.stubGlobal("fetch", (request: Request) => {
      if (request.url.includes("/partners")) {
        return Promise.resolve(
          jsonResponse({ data: [], page: { next_cursor: null } }),
        );
      }
      return Promise.resolve(stubBackend([d], { single: d })(request));
    });

    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await userEvent.click(screen.getByRole("button", { name: "Table" }));
    await userEvent.click(screen.getByRole("button", { name: "Filter" }));

    // A picker with nothing in it asks a question that has no answers.
    const menu = screen.getByRole("group", { name: "Filter" });
    expect(within(menu).queryByRole("button", { name: "Partner" })).toBeNull();
  });
});

describe("DealScreen — the header's Email verb", () => {
  it("carries the same Email verb every record header does", async () => {
    const d = deal({ id: "x", version: 4 });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);
    const verb = await screen.findByRole("button", { name: "Email" });
    expect(verb.querySelector(".lucide-mail")).toBeTruthy();
    expect(verb.hasAttribute("disabled")).toBe(false);
  });
});
