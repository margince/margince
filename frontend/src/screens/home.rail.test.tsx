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
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { HomeScreen } from "./home";
import { readingsDay } from "./home.fixtures";
import type { Deal } from "./home.queries";

// Home's context rail (screens/home.rail.tsx): what the night shift did, what
// the pipeline is worth, and what has gone quiet. Three panels, all of them
// READ, and each one gated on its OWN query — which is the property these cases
// exist to hold: a transient failure in one panel must never blank another, and
// a panel with no answer yet must draw nothing rather than a row of zeros.
//
// Split out of home.test.tsx at the 1000-line ceiling (frontend/CLAUDE.md), on
// the seam the screen itself is built along: the work column and its readings
// are that file, the rail beside them is this one. The stub harness is spelled
// again here rather than shared, the same way every screen suite in this tree
// carries its own — a test file that imported another test file would run its
// neighbour's cases a second time.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const emptyPage = { data: [], page: { next_cursor: null, has_more: false } };

/** One request the screen made, as the route it names and what it carried. */
type Call = { method: string; path: string; body: unknown };

type Routes = Record<string, (body: unknown) => Response | Promise<Response>>;

// Every read Home fans out to, answered honestly by default so each case
// declares only the route it is about: a session, no nightly digest, no brief
// run, and a pipeline report with no rows. The report matters — the fallback
// empty PAGE carries no `rows`, which the pipeline reading would read as a
// failure and put a refusal in the rail of every case in this file.
const DEFAULTS: Routes = {
  "GET /me": () => jsonResponse(meFixture()),
  "GET /brief": () => jsonResponse({ title: "Not Found" }, 404),
  "GET /digest": () =>
    jsonResponse({ title: "Not Found", code: "no_digest_yet" }, 404),
  "POST /reports/deals-by-stage": () =>
    jsonResponse({ report: "deals-by-stage", plan: {}, columns: [], rows: [] }),
  // Same reason as the report above: the fallback empty PAGE carries no
  // `readings` and no `counts`, and the Brief's strip reads both as required
  // fields. An unrouted worklist read has to answer with a worklist.
  "GET /worklist": () => jsonResponse(readingsDay({}, [])),
};

/**
 * Routes the stubbed fetch by method+path and RECORDS every call: the pipeline
 * report's FILTER is only visible in the request, and no rendering of the
 * figures it returns can prove the screen asked for open deals.
 */
function stubApi(routes: Routes): Call[] {
  const calls: Call[] = [];
  const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(
      request ? request.url : String(input),
      "https://test.local",
    );
    const method = request?.method ?? init?.method ?? "GET";
    const path = url.pathname.replace(/^\/v1/, "");
    let body: unknown = null;
    if (method !== "GET") {
      try {
        // `clone()`: the client sends a Request and the handler below may read
        // the same body again.
        body = request
          ? await request.clone().json()
          : JSON.parse(String(init?.body));
      } catch {
        // A write with no body at all is not a malformed one.
        body = null;
      }
    }
    calls.push({ method, path, body });
    const handler =
      routes[`${method} ${path}`] ?? DEFAULTS[`${method} ${path}`];
    return handler ? handler(body) : jsonResponse(emptyPage);
  });
  vi.stubGlobal("fetch", mock);
  return calls;
}

const fleetDeal: Deal = {
  id: "d-1",
  name: "Fleet retrofit",
  amount_minor: 4_800_000,
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: "s2",
  status: "open",
  stalled: false,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-05-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const quietDeal: Deal = {
  ...fleetDeal,
  id: "d-9",
  name: "Ostwind refit",
  amount_minor: 1_200_000,
  organization_id: "org-9",
  stalled: true,
};

// ── The context rail ──

describe("HomeScreen — the context rail", () => {
  // Home used to pass `org: ""` for every card here, so every quiet deal on
  // this page claimed to belong to no company at all. The panel resolves the
  // company through the same naming the pipeline board uses.
  it("names the company on a quiet deal", async () => {
    stubApi({
      "GET /deals": () => jsonResponse({ data: [fleetDeal, quietDeal] }),
      "GET /organizations/org-9": () =>
        jsonResponse({ id: "org-9", display_name: "Nordwind Logistik" }),
    });
    render(<HomeScreen />);

    const card = await screen.findByText("Ostwind refit");
    const panel = card.closest("a");
    expect(panel).toBeTruthy();
    // Awaited, not read: the company name comes from a second read that the
    // card does not wait for, so it can still be in flight when the deal's own
    // title is already on screen.
    expect(
      await within(panel ?? card).findByText("Nordwind Logistik"),
    ).toBeTruthy();
    // The quiet deal is on the page as a CARD rather than as a count. The
    // briefing line that carried "1 has gone quiet" is gone: the panel listing
    // the deal itself says more than a figure above it could, and says it in
    // the one place a reader can act on it.
    expect(within(panel ?? card).getByText("Ostwind refit")).toBeTruthy();
  });

  it("says so when nothing has gone quiet", async () => {
    stubApi({ "GET /deals": () => jsonResponse({ data: [fleetDeal] }) });
    render(<HomeScreen />);

    expect(await screen.findByText("Nothing has gone quiet.")).toBeTruthy();
  });

  const digestBase = {
    date: "2026-07-16",
    generated_at: "2026-07-17T03:00:00Z",
    capture: {
      messages_synced: 42,
      activities_created: 42,
      people_created: 5,
      organizations_created: 2,
    },
    review: {
      dedupe_open: 3,
      approvals_pending: 1,
      classify: { commitments: 4, meetings: 2, noise: 30 },
    },
  };

  it("renders the overnight counts and jumps into the duplicates queue", async () => {
    stubApi({
      "GET /digest": () => jsonResponse({ ...digestBase, connectors: [] }),
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    // Waited on CONTENT, not on the panel's name: the name is also what the
    // pending state announces now that a wait says what it is waiting for, so
    // finding it proves the panel exists rather than that the digest arrived.
    await screen.findByText("Emails synced");
    expect(screen.getByText("People created")).toBeTruthy();
    expect(screen.getByText("Companies created")).toBeTruthy();
    expect(
      screen.getByText(
        "Classified overnight: 4 commitments · 2 meetings · 30 noise",
      ),
    ).toBeTruthy();
    await user.click(screen.getByText("Duplicates to review"));
    expect(window.location.hash).toBe("#/worklist");
  });

  // /digest is a specified operation an installation may not implement yet, so
  // the honest answer is 501 — and a refusal is not a delay. Read as an error,
  // the client retried it, and React Query pauses between retries while the tab
  // is hidden: the panel carried skeleton bars that would still be there the
  // next day.
  it("renders no overnight panel for a 501, and no loading block either", async () => {
    stubApi({
      "GET /digest": () =>
        jsonResponse(
          {
            title: "Not Implemented",
            code: "not_implemented",
            detail:
              "operation GetMorningDigest is specified but not yet implemented",
          },
          501,
        ),
    });
    render(<HomeScreen />);

    await screen.findByRole("region", { name: en["brief.feed.title"] });
    // The rail's own reads settle after the feed's region appears, so this
    // waits for the page to go quiet rather than asserting into a page still
    // in flight.
    await waitFor(() =>
      expect(document.querySelector("[aria-busy='true']")).toBeNull(),
    );
    expect(screen.queryByRole("heading", { name: "Overnight" })).toBeNull();
    expect(screen.queryByText(/couldn't load/i)).toBeNull();
    expect(document.body.textContent).not.toContain("not yet implemented");
  });

  it("renders no overnight panel at all before the first nightly run", async () => {
    stubApi({});
    render(<HomeScreen />);

    // The rail's reads must have ANSWERED before absence means anything. The
    // feed's region is drawn on the first paint, so awaiting it proves only
    // that the page mounted — an assertion made against it would pass over a
    // panel that renders unconditionally, which is the defect being excluded.
    await waitFor(() =>
      expect(document.querySelector("[aria-busy='true']")).toBeNull(),
    );
    expect(screen.queryByRole("heading", { name: "Overnight" })).toBeNull();
  });

  // The one place connector health reaches a reader without visiting Settings.
  // A degraded source is news, in Settings' own vocabulary, and it jumps to
  // where a reader's mailboxes actually live.
  it("names an unhealthy source and jumps to Settings → Connections", async () => {
    stubApi({
      "GET /digest": () =>
        jsonResponse({
          ...digestBase,
          connectors: [
            {
              provider: "gmail",
              status: "reauth_required",
              last_sync_error_class: "auth",
            },
          ],
        }),
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    expect(await screen.findByText(/rejected our credentials/i)).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: "Fix the connection" }),
    );
    expect(window.location.hash).toBe("#/settings/connections");
  });

  it("stays quiet when every source is healthy — a green row is noise", async () => {
    stubApi({
      "GET /digest": () =>
        jsonResponse({
          ...digestBase,
          connectors: [{ provider: "gmail", status: "connected" }],
        }),
    });
    render(<HomeScreen />);

    await screen.findByText("Overnight");
    expect(
      screen.queryByRole("button", { name: "Fix the connection" }),
    ).toBeNull();
  });

  it("lists the projects that moved or went quiet, each linking to its page", async () => {
    const projectId = "01a00000-0000-7000-8000-000000000001";
    stubApi({
      "GET /projects/01a00000-0000-7000-8000-000000000001": () =>
        jsonResponse({ id: projectId, name: "ERP replacement" }),
      "GET /digest": () =>
        jsonResponse({
          ...digestBase,
          connectors: [],
          projects: {
            phase_changes: [
              {
                project_id: projectId,
                name: "ERP replacement",
                from_phase: "pursuing",
                to_phase: "delivering",
                occurred_at: "2026-07-17T01:00:00Z",
              },
            ],
            new_commitments: [],
            gone_quiet: [
              {
                project_id: projectId,
                name: "ERP replacement",
                phase: "delivering",
                quiet_since: "2026-06-07T01:00:00Z",
                days_quiet: 40,
              },
            ],
          },
        }),
    });
    const user = userEvent.setup();
    render(<HomeScreen />);

    const moves = await screen.findByLabelText("Phase moves");
    expect(moves.textContent).toContain("Pursuing → Delivering");
    expect(screen.getByLabelText("Gone quiet").textContent).toContain(
      "quiet for 40 days",
    );
    const links = await screen.findAllByRole("button", {
      name: "ERP replacement",
    });
    expect(links.length).toBe(2);
    await user.click(links[0]);
    expect(window.location.hash).toBe(`#/projects/${projectId}`);
  });

  it("renders no projects block when the digest carries no section", async () => {
    stubApi({
      "GET /digest": () => jsonResponse({ ...digestBase, connectors: [] }),
    });
    render(<HomeScreen />);

    await screen.findByText("Overnight");
    expect(screen.queryByLabelText("Phase moves")).toBeNull();
  });
});

// The open pipeline is grouped by currency and rendered one line each rather
// than summed: adding native minor units across currencies produces a number
// that is not money.
describe("HomeScreen — the open pipeline", () => {
  it("shows the server's raw and weighted totals", async () => {
    stubApi({
      "POST /reports/deals-by-stage": () =>
        jsonResponse({
          report: "deals-by-stage",
          plan: {},
          columns: [],
          rows: [
            {
              currency: "EUR",
              deals: 12,
              raw_minor: 9_900_000,
              weighted_minor: 3_300_000,
            },
          ],
        }),
    });
    render(<HomeScreen />);

    expect(await screen.findByText("€99,000.00")).toBeTruthy();
    expect(screen.getByText("€33,000.00 weighted")).toBeTruthy();
    expect(screen.getByText("12 open deals")).toBeTruthy();
  });

  it("gives each currency its own line rather than one meaningless sum", async () => {
    stubApi({
      "POST /reports/deals-by-stage": () =>
        jsonResponse({
          report: "deals-by-stage",
          plan: {},
          columns: [],
          rows: [
            {
              currency: "EUR",
              deals: 2,
              raw_minor: 100_000,
              weighted_minor: 40_000,
            },
            {
              currency: "USD",
              deals: 3,
              raw_minor: 200_000,
              weighted_minor: 50_000,
            },
          ],
        }),
    });
    render(<HomeScreen />);

    expect(await screen.findByText("€1,000.00")).toBeTruthy();
    expect(screen.getByText("US$2,000.00")).toBeTruthy();
    // No combined figure anywhere: 300_000 minor units is not a currency.
    expect(screen.queryByText("€3,000.00")).toBeNull();
  });

  // The filter is load-bearing: the report's own base predicate is
  // unarchived-only, so without status=open this headline would count won and
  // lost deals and grow every time somebody closed something. Asserting the
  // rendered numbers cannot catch that — only the request can.
  it("asks for open deals only, grouped by currency", async () => {
    const calls = stubApi({});
    render(<HomeScreen />);

    await waitFor(() =>
      expect(
        calls.some((call) => call.path === "/reports/deals-by-stage"),
      ).toBe(true),
    );
    const body = calls.find(
      (call) => call.path === "/reports/deals-by-stage",
    )?.body;
    expect(body).toMatchObject({
      filters: { status: "open" },
      group_by: ["currency"],
      aggregates: [
        { fn: "count", as: "deals" },
        { fn: "sum", field: "amount_minor", as: "raw_minor" },
        { fn: "sum", field: "weighted_amount_minor", as: "weighted_minor" },
      ],
    });
  });

  // A refusal is not an absence. An empty panel would read as "there is no
  // pipeline", which is a claim about the data made in place of a claim about
  // authority.
  it("keeps its place and says so when the figure cannot be read", async () => {
    stubApi({
      "POST /reports/deals-by-stage": () =>
        jsonResponse({ title: "Forbidden" }, 403),
    });
    render(<HomeScreen />);

    expect(
      await screen.findByText("This figure could not be loaded."),
    ).toBeTruthy();
  });

  it("says when a mask has kept deals out of the figures", async () => {
    stubApi({
      "POST /reports/deals-by-stage": () =>
        jsonResponse({
          report: "deals-by-stage",
          plan: {},
          columns: [],
          excluded_by_permission: 4,
          rows: [
            {
              currency: "EUR",
              deals: 1,
              raw_minor: 100_000,
              weighted_minor: 40_000,
            },
          ],
        }),
    });
    render(<HomeScreen />);

    expect(
      await screen.findByText(
        "4 deals are not in these figures — your access does not cover them.",
      ),
    ).toBeTruthy();
    // And the singular reads as one deal, not "1 open deals".
    expect(screen.getByText("1 open deal")).toBeTruthy();
  });

  it("draws no position panel at all when there is no open pipeline", async () => {
    stubApi({});
    render(<HomeScreen />);

    // Waits for the reads to answer, for the reason the overnight case above
    // gives: absence proves nothing until the read that would have filled it
    // has come back.
    await waitFor(() =>
      expect(document.querySelector("[aria-busy='true']")).toBeNull(),
    );
    expect(screen.queryByText("Position")).toBeNull();
  });
});
