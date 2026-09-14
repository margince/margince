/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { DealMailAside } from "./dealmailaside";

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
