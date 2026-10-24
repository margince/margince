/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { ADDRESSABLE_DEAL_DIALS, type DealDial } from "../../lib/dealsaddress";
import { DealsScreen, dealsQueryParams } from "./deals";

// A list opened from an address shows the view the address names.
//
// The company page's lost figure and the pipeline report's open count both
// write `#/deals?company_id=…&status=…`. A link opened in a new tab has no
// picked label, so the list sends every dial the address names. The chip names
// its record as a picked one does.

type Deal = components["schemas"]["Deal"];

const COMPANY = { id: "c-nw", display_name: "Northwind Traders" };

// A company the API will not admit exists: gone, or hidden by row scope.
const HIDDEN_COMPANY_ID = "c-hidden";

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
  };
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <DealsScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

type Seen = { dealUrls: string[]; reportBodies: { filters?: unknown }[] };

const noneSeen = (): Seen => ({ dealUrls: [], reportBodies: [] });

/** The backend this screen reads; `dealUrls` and `reportBodies` record asks. */
function stubBackend(seen: Seen) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    const path = new URL(url, "http://localhost").pathname;
    if (url.includes("/pipelines")) {
      return jsonResponse({
        data: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages: [
              {
                id: "s1",
                pipeline_id: "pl",
                name: "Qualify",
                position: 1,
                semantic: "open",
                win_probability: 20,
              },
            ],
          },
        ],
        page: { next_cursor: null },
      });
    }
    if (method === "POST" && url.includes("/reports/deals-by-stage")) {
      seen.reportBodies.push(request ? await request.json() : init?.body);
      return jsonResponse({
        report: "deals-by-stage",
        base_currency: "EUR",
        as_of: "2026-10-01",
        plan: {},
        columns: [],
        rows: [],
      });
    }
    if (url.includes("/me")) {
      return jsonResponse({
        user: {
          id: "u-me",
          email: "me@acme.test",
          display_name: "Me",
          status: "active",
          is_agent: false,
        },
        roles: ["rep"],
        teams: [],
      });
    }
    if (path.endsWith(`/companies/${COMPANY.id}`)) {
      return jsonResponse(COMPANY);
    }
    if (path.endsWith(`/companies/${HIDDEN_COMPANY_ID}`)) {
      return jsonResponse({ title: "Not found", status: 404 }, 404);
    }
    if (path.endsWith("/companies")) {
      return jsonResponse({ data: [COMPANY], page: { next_cursor: null } });
    }
    if (path.endsWith("/deals")) {
      seen.dealUrls.push(url);
      return jsonResponse({
        data: [deal({})],
        page: { next_cursor: null, has_more: false },
      });
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

// A link may only name a dial the list sends on. Each sample is typed by the
// dial union, so a new dial without one does not compile. A dial the list
// ignores fails the case below.
const SAMPLE: Record<DealDial, string> = {
  stage_id: "s1",
  company_id: "c-nw",
  status: "lost",
  forecast_category: "commit",
};

describe("a deals link", () => {
  it.each(ADDRESSABLE_DEAL_DIALS)(
    "names %s, which the list sends on",
    (dial) => {
      const sent = dealsQueryParams({
        pipelineId: "",
        q: "",
        sort: "",
        includeArchived: false,
        filters: { [dial]: SAMPLE[dial] },
      });
      expect(sent[dial]).toBe(SAMPLE[dial]);
    },
  );
});

describe("the deals list opened from an address", () => {
  it("narrows by the status the address names, and says so", async () => {
    window.location.hash = "#/deals?status=lost&view=table";
    const seen = noneSeen();
    vi.stubGlobal("fetch", stubBackend(seen));
    render();

    // The row names the dial and its value, so the narrowing is on screen
    // rather than only in the address.
    expect(
      await screen.findByRole("group", { name: "Status: Lost" }),
    ).toBeTruthy();
    await waitFor(() =>
      expect(seen.dealUrls.at(-1)?.includes("status=lost")).toBe(true),
    );
  });

  it.each([
    ["status", "closed", "Status"],
    ["forecast_category", "slipped", "Forecast"],
  ])(
    "drops %s=%s, which the wire does not admit, from row and request",
    async (dial, value, chip) => {
      window.location.hash = `#/deals?${dial}=${value}&view=table`;
      const seen = noneSeen();
      vi.stubGlobal("fetch", stubBackend(seen));
      render();

      await waitFor(() => expect(seen.dealUrls.length).toBeGreaterThan(0));
      expect(seen.dealUrls.at(-1)?.includes(`${dial}=`)).toBe(false);
      expect(
        screen.queryByRole("group", { name: new RegExp(`^${chip}:`) }),
      ).toBeNull();
    },
  );

  it("widens when the status row is deleted", async () => {
    window.location.hash = "#/deals?status=open&view=table";
    const seen = noneSeen();
    vi.stubGlobal("fetch", stubBackend(seen));
    const user = userEvent.setup();
    render();

    const row = await screen.findByRole("group", { name: "Status: Open" });
    await user.click(
      within(row).getByRole("button", {
        name: "More actions for the Status filter",
      }),
    );
    await user.click(screen.getByRole("button", { name: "Delete filter" }));

    await waitFor(() =>
      expect(seen.dealUrls.at(-1)?.includes("status=")).toBe(false),
    );
  });

  it("gives the board's stage totals the status it is narrowed by", async () => {
    window.location.hash = "#/deals?status=open";
    const seen = noneSeen();
    vi.stubGlobal("fetch", stubBackend(seen));
    render();

    await waitFor(() => expect(seen.reportBodies.length).toBeGreaterThan(0));
    expect(seen.reportBodies.at(-1)).toMatchObject({
      filters: { status: "open" },
    });
  });

  it("names the company chip by its name, not its id", async () => {
    window.location.hash = `#/deals?company_id=${COMPANY.id}&view=table`;
    const seen = noneSeen();
    vi.stubGlobal("fetch", stubBackend(seen));
    render();

    expect(
      await screen.findByRole("group", {
        name: `Company: ${COMPANY.display_name}`,
      }),
    ).toBeTruthy();
    expect(screen.queryByText(COMPANY.id)).toBeNull();
  });

  it("keeps a company the reader cannot open applied, named as withheld", async () => {
    window.location.hash = `#/deals?company_id=${HIDDEN_COMPANY_ID}&view=table`;
    const seen = noneSeen();
    vi.stubGlobal("fetch", stubBackend(seen));
    render();

    expect(
      await screen.findByRole("group", { name: "Company: Company withheld" }),
    ).toBeTruthy();
    expect(screen.queryByText(HIDDEN_COMPANY_ID)).toBeNull();
    await waitFor(() =>
      expect(
        seen.dealUrls.at(-1)?.includes(`company_id=${HIDDEN_COMPANY_ID}`),
      ).toBe(true),
    );
  });
});
