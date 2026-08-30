/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { SignalsSection } from "./companyrail";

// A newsroom signal CITES its article rather than copying it, so the headline
// on the row is the whole of what this product holds. Without the address the
// citation proves nothing and a reader who wants the announcement has nowhere
// to go — which is what this file exists to keep from happening again.
//
// Its own file rather than companyrail.test.tsx's: that one is already past
// the 1000-line ceiling test files split at.

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
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

function signal(overrides: Record<string, unknown> = {}) {
  return {
    id: "s-1",
    kind: "funding",
    source_channel: "web",
    resolution_state: "resolved",
    severity: "info",
    summary: "Brandt Automotive raises a Series B",
    evidence: [],
    status: "open",
    detected_at: "2026-06-01T08:00:00Z",
    source: "newsroom",
    captured_by: "system",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
    ...overrides,
  };
}

function stubSignals(rows: readonly unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      jsonResponse({
        data: rows,
        page: { has_more: false, next_cursor: null },
      }),
    ),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("SignalsSection", () => {
  it("links the page a signal was read off", async () => {
    stubSignals([
      signal({
        evidence: [
          {
            snippet: "Brandt Automotive raises a Series B",
            source_type: "page",
            source_id: "https://brandt.example/news/series-b",
          },
        ],
      }),
    ]);

    render(<SignalsSection orgId="o-1" />);

    const link = await screen.findByRole("link", {
      name: "Read the announcement",
    });
    expect(link).toHaveAttribute(
      "href",
      "https://brandt.example/news/series-b",
    );
    // Opened away from the app, and without handing the destination this
    // record's address in a Referer. Asserted as TOKENS rather than as a
    // substring: "notnoreferrer" contains "noreferrer" and is not a relation
    // any browser honours, so a substring check passes a mutation that
    // restores both window.opener access and the Referer leak.
    expect(link).toHaveAttribute("target", "_blank");
    const rel = (link.getAttribute("rel") ?? "").split(/\s+/);
    expect(rel).toContain("noopener");
    expect(rel).toContain("noreferrer");
  });

  it("links only the row whose citation is a page", async () => {
    // Both rows render together, so "no link here" is proved against a link
    // that DOES appear in the same pass — otherwise the assertion passes just
    // as well when the component is deleted outright.
    stubSignals([
      signal({
        id: "s-internal",
        summary: "Two meetings booked and none held",
        evidence: [
          {
            snippet: "Two meetings booked and none held",
            source_type: "activity",
            source_id: "01a04fdf-7a3c-75f6-bdf6-5f868ea3a705",
          },
        ],
      }),
      signal({
        id: "s-news",
        summary: "Brandt Automotive raises a Series B",
        evidence: [
          {
            snippet: "Brandt Automotive raises a Series B",
            source_type: "page",
            source_id: "https://brandt.example/news/series-b",
          },
        ],
      }),
    ]);

    render(<SignalsSection orgId="o-1" />);

    // Exactly one link, and it belongs to the row that cited a page.
    const links = await screen.findAllByRole("link", {
      name: "Read the announcement",
    });
    expect(links).toHaveLength(1);
    const row = links[0].closest(".co-signal-row");
    expect(row?.textContent).toContain("Brandt Automotive raises a Series B");
    expect(row?.textContent).not.toContain("Two meetings booked");
  });

  it("reaches past a malformed citation to a usable one", async () => {
    // `.find` stopping at the first entry that merely CLAIMS to be a page
    // would hide the reachable address behind it, and the row would fall
    // silently back to no link at all.
    stubSignals([
      signal({
        evidence: [
          {
            snippet: "…",
            source_type: "page",
            source_id: "javascript:alert(1)",
          },
          {
            snippet: "Brandt Automotive raises a Series B",
            source_type: "page",
            source_id: "https://brandt.example/news/series-b",
          },
        ],
      }),
    ]);

    render(<SignalsSection orgId="o-1" />);

    const link = await screen.findByRole("link", {
      name: "Read the announcement",
    });
    expect(link.getAttribute("href")).toBe(
      "https://brandt.example/news/series-b",
    );
  });

  it("survives an evidence shape it cannot walk", async () => {
    // The client validates no response body, so a server ahead of this tab can
    // send a shape `.find` would throw on — and a throw here blanks the whole
    // account page over one row's citation.
    stubSignals([
      signal({ summary: "Shape from a newer server", evidence: {} }),
    ]);

    render(<SignalsSection orgId="o-1" />);

    expect(
      await screen.findByText("Shape from a newer server"),
    ).toBeInTheDocument();
  });

  it("draws no anchor at all when every citation is unusable", async () => {
    // A scheme that would execute on click never reaches an href, and nothing
    // is left in its place: an anchor with no destination is worse than none.
    stubSignals([
      signal({
        evidence: [
          {
            snippet: "Brandt Automotive raises a Series B",
            source_type: "page",
            source_id: "javascript:alert(1)",
          },
        ],
      }),
    ]);

    render(<SignalsSection orgId="o-1" />);

    expect(
      await screen.findByText("Brandt Automotive raises a Series B"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Read the announcement")).not.toBeInTheDocument();
  });
});
