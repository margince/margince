/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider, useT } from "../i18n";
import { DealMailAside, lastMailColumn } from "./dealmailaside";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DAY_MS = 86_400_000;

function daysAgo(days: number): string {
  return new Date(Date.now() - days * DAY_MS).toISOString();
}

function mail(
  id: string,
  subject: string | null,
  direction: "inbound" | "outbound" | null,
  days: number,
  withheld = false,
) {
  return {
    id,
    kind: "email",
    subject: withheld ? null : subject,
    direction,
    occurred_at: daysAgo(days),
    content_state: withheld ? "withheld" : "available",
    source: "gmail",
    captured_by: "connector:gmail",
    created_at: daysAgo(days),
    updated_at: daysAgo(days),
  };
}

// Answers the one read the aside makes, and records what it asked for.
function serve(rows: unknown[], status = 200) {
  const seen: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      seen.push(String(input instanceof Request ? input.url : input));
      return new Response(
        JSON.stringify(
          status === 200
            ? { data: rows, page: { next_cursor: null } }
            : { code: "internal", message: "no timeline today" },
        ),
        {
          status,
          headers: {
            "Content-Type":
              status === 200 ? "application/json" : "application/problem+json",
          },
        },
      );
    }),
  );
  return seen;
}

function render(dealId = "d1") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <DealMailAside dealId={dealId} href={`#/deals/${dealId}`} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("DealMailAside", () => {
  // The rows are the deal's own emails, newest first as the timeline sends
  // them, each cited by subject and by which way it went and how long ago.
  it("cites the deal's last emails with their direction and age", async () => {
    const seen = serve([
      mail("a1", "AW: Ausbildungsoffensive Bayern", "outbound", 10),
      mail("a2", "AW: RetrieverClub MVP", "inbound", 61),
      mail("a3", "Logged by hand", null, 3),
    ]);
    render();
    expect(
      await screen.findByText("AW: Ausbildungsoffensive Bayern"),
    ).toBeTruthy();
    expect(screen.getByText("Sent 10 d ago")).toBeTruthy();
    expect(screen.getByText("Received 61 d ago")).toBeTruthy();
    // A logged email that named no direction is dated and nothing more.
    expect(screen.getByText("3 d ago")).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "View all activity" })
        .getAttribute("href"),
    ).toBe("#/deals/d1");
    // ONE read, narrowed to this deal's mail and capped: the flyout is an
    // aside, and an aside that paged the whole timeline would be the page.
    expect(seen).toHaveLength(1);
    const url = new URL(seen[0] ?? "", "http://test");
    expect(url.searchParams.get("entity_type")).toBe("deal");
    expect(url.searchParams.get("entity_id")).toBe("d1");
    expect(url.searchParams.get("kind")).toBe("email");
    expect(url.searchParams.get("limit")).toBe("3");
  });

  // A message this reader may know about but not read is cited in the
  // timeline's words for that, and never by a subject the wire withheld.
  it("says a held message is not shared, and offers nothing to press on it", async () => {
    serve([mail("a1", "Board pack", "inbound", 1, true)]);
    render();
    expect(await screen.findByText("Not shared with you")).toBeTruthy();
    expect(screen.queryByText("Board pack")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("says so when the reader may know about no mail on the deal", async () => {
    serve([]);
    render();
    expect(await screen.findByText("No email on this deal yet")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "View all activity" }),
    ).toBeTruthy();
  });

  // A failed read is a failed read, in the shared spelling every other
  // query-backed surface uses — never an empty list that reads as "no mail".
  it("reports a failed read rather than an empty exchange", async () => {
    serve([], 500);
    render();
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.queryByText("No email on this deal yet")).toBeNull();
  });
});

type Deal = components["schemas"]["Deal"];

function tableDeal(overrides: Partial<Deal>): Deal {
  return {
    id: "d1",
    name: "Fleet retrofit",
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

// The column needs a translator, which only a component can ask for.
function Cell({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  return <>{lastMailColumn(t).cell(deal)}</>;
}

describe("lastMailColumn", () => {
  // The table's cell is the card's chip: same fact, same flyout trigger, off
  // the same wire field — so a reader switching views reads one thing.
  it("draws the chip for a deal with mail, and says so for one without", () => {
    serve([]);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { unmount } = rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <Cell
            deal={tableDeal({
              last_email: { occurred_at: daysAgo(10), direction: "outbound" },
            })}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(screen.getByRole("button", { name: /Last email/ })).toBeTruthy();
    expect(screen.getByText("10 d ago")).toBeTruthy();
    unmount();

    rtlRender(
      <LocaleProvider initial="en">
        <Cell deal={tableDeal({})} />
      </LocaleProvider>,
    );
    expect(screen.getByText("no email yet")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  // The header offers no ordering: the field is attached after the query, and
  // a sortable header on it would be a control that does nothing.
  it("offers no sort", () => {
    let column: ReturnType<typeof lastMailColumn> | undefined;
    function Probe() {
      column = lastMailColumn(useT());
      return null;
    }
    rtlRender(
      <LocaleProvider initial="en">
        <Probe />
      </LocaleProvider>,
    );
    expect(column?.sort).toBeUndefined();
    expect(column?.header).toBe("Last email");
  });
});
