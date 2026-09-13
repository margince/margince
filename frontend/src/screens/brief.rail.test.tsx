/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { BriefScreen } from "./brief";
import { readingsDay } from "./brief.fixtures";
import type { Deal } from "./brief.queries";
import { OvernightPanel } from "./brief.rail.overnight";
import type { WorklistItem } from "./worklist.queries";

// Context panels keep their own loading and failure states so a failed source
// cannot erase an independently available answer.

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

// Every read Brief fans out to, answered honestly by default so each case
// declares only the route it is about: a session, no nightly digest, and no
// brief run. The deals-by-stage report is NOT here any more — Brief stopped
// asking for it when the open-pipeline panel left the rail, and a default route
// for a read nobody makes is a stub that outlives its caller.
const DEFAULTS: Routes = {
  "GET /me": () => jsonResponse(meFixture()),
  "GET /brief": () => jsonResponse({ title: "Not Found" }, 404),
  "GET /digest": () =>
    jsonResponse({ title: "Not Found", code: "no_digest_yet" }, 404),
  // The fallback empty PAGE carries no
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

// ── The context rail ──

describe("BriefScreen — the context rail", () => {
  // Brief used to pass `company: ""` for every card here, so every quiet deal on
  // this page claimed to belong to no company at all. The panel resolves the
  // company through the same naming the pipeline board uses.
  it("uses the scoped queue for remaining risks and never reads the deals list", async () => {
    const calls = stubApi({
      "GET /worklist": () =>
        jsonResponse(
          readingsDay({}, [
            ...Array.from({ length: 5 }, (_, i) =>
              taskRow(`t${i}`, `Follow up ${i}`),
            ),
            {
              id: "risk-1",
              source: "deal_at_risk",
              category: "deals_at_risk",
              level: 3,
              title: "Ostwind refit",
              consequence: "deal_drifts",
              because: [],
              actions: [],
            },
          ]),
        ),
    });
    render(<BriefScreen />);
    expect(await screen.findByText("Ostwind refit")).toBeTruthy();
    expect(calls.some((call) => call.path === "/deals")).toBe(false);
  });

  // A clear watch list is not a panel. The box, the header band and the
  // hairline stood around one grey sentence beside the panels that did have
  // news, so the news was read last. The sentence moves to the rail's quiet
  // panel — the RailQuiet cases at the foot of this file.
  it("draws no watch panel when nothing has gone quiet", async () => {
    stubApi({ "GET /deals": () => jsonResponse({ data: [fleetDeal] }) });
    render(<BriefScreen />);

    await waitFor(() =>
      expect(document.querySelector("[aria-busy='true']")).toBeNull(),
    );
    expect(document.querySelector("#brief-watch")).toBeNull();
  });

  // The panel keeps its box wherever it has something to SAY. Past the end of
  // Brief's one page an empty list is "nothing on the page we read" rather than
  // "nothing has gone quiet", and that caveat is content.
  it("does not duplicate risks in a second panel on a partial page", async () => {
    stubApi({
      "GET /worklist": () =>
        jsonResponse({ ...readingsDay({}, []), next_cursor: "next" }),
    });
    const { container } = render(<BriefScreen />);
    await screen.findByText(en["brief.feed.title"]);
    expect(container.querySelector("#brief-watch")).toBeNull();
  });

  const digestBase = {
    date: "2026-07-16",
    generated_at: "2026-07-17T03:00:00Z",
    capture: {
      messages_synced: 42,
      activities_created: 42,
      contacts_created: 5,
      companies_created: 2,
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
    render(<BriefScreen />);

    // Waited on CONTENT, not on the panel's name: the name is also what the
    // pending state announces now that a wait says what it is waiting for, so
    // finding it proves the panel exists rather than that the digest arrived.
    await screen.findByText("Emails synced");
    expect(screen.getByText("Contacts created")).toBeTruthy();
    expect(screen.getByText("Companies created")).toBeTruthy();
    expect(
      screen.getByText(
        "Classified: 4 promises, 2 meetings, 30 messages without a sales action.",
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
    render(<BriefScreen />);

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
    render(<BriefScreen />);

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
    render(<BriefScreen />);

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
    render(<BriefScreen />);

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
    render(<BriefScreen />);

    const moves = await screen.findByLabelText("Phase moves");
    expect(moves.textContent).toContain("Pursuing → Delivering");
    // By ROLE as well as by name: the watch panel in the rail carries the same
    // words as its heading, and this assertion is about the digest's list.
    expect(
      screen.getByRole("list", { name: "Gone quiet" }).textContent,
    ).toContain("quiet for 40 days");
    const links = await screen.findAllByRole("link", {
      name: "ERP replacement",
    });
    expect(links.length).toBe(2);
    // The href is the destination, and it is also what a new tab and a
    // middle-click follow — neither of which goes through an onClick.
    expect(links[0].getAttribute("href")).toBe(`#/projects/${projectId}`);
  });

  it("renders no projects block when the digest carries no section", async () => {
    stubApi({
      "GET /digest": () => jsonResponse({ ...digestBase, connectors: [] }),
    });
    render(<BriefScreen />);

    await screen.findByText("Overnight");
    expect(screen.queryByLabelText("Phase moves")).toBeNull();
  });
});

// ── When a source has nothing to report ──
//
// Four panels of chrome around four grey sentences is what a clear morning used
// to draw, and it cost the panels that DID have news the reader's eye. Each
// panel now collapses on its empty reading and one quiet panel carries a line
// per silent source. The two halves have to agree: a printed "nothing booked"
// over a schedule panel that drew would be worse than either alone, which is
// why the panels and this panel share one predicate each.

function taskRow(id: string, title: string): WorklistItem {
  return {
    id,
    source: "task",
    level: 2,
    category: "tasks",
    title,
    because: [],
    consequence: "task_slips",
    actions: ["complete", "open"],
  };
}

describe("the overnight panel with no digest", () => {
  // /v1/digest answers 404 before the first nightly run and 501 where the
  // installation does not implement it. Both mean the same thing to a reader,
  // and the panel draws NOTHING for either — not a row of zeros, which a reader
  // cannot tell from a real count, and not a skeleton, which never resolves.
  for (const [status, code] of [
    [404, "no_digest_yet"],
    [501, "not_implemented"],
  ] as const) {
    it(`renders nothing at all for a ${status}`, async () => {
      stubApi({
        "GET /digest": () => jsonResponse({ title: "Absent", code }, status),
      });
      const { container } = render(<OvernightPanel />);

      await waitFor(() =>
        expect(container.querySelector("[aria-busy='true']")).toBeNull(),
      );
      expect(container.innerHTML).toBe("");
    });
  }
});
