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
    // record's address in a Referer.
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noreferrer"));
  });

  it("offers no source link when nothing was read off a page", async () => {
    // A signal derived from this product's own records cites a row, not a web
    // address. An "open the source" link pointing at an internal id would send
    // a reader nowhere.
    stubSignals([
      signal({
        summary: "Two meetings booked and none held",
        evidence: [
          {
            snippet: "Two meetings booked and none held",
            source_type: "activity",
            source_id: "01a04fdf-7a3c-75f6-bdf6-5f868ea3a705",
          },
        ],
      }),
    ]);

    render(<SignalsSection orgId="o-1" />);

    expect(
      await screen.findByText("Two meetings booked and none held"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Read the announcement" }),
    ).not.toBeInTheDocument();
  });

  it("renders the headline as text when the cited address is not a web page", async () => {
    // A stored value that is not an http(s) URL degrades to no link rather
    // than to a dead anchor a reader would click.
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
    expect(
      screen.queryByRole("link", { name: "Read the announcement" }),
    ).not.toBeInTheDocument();
  });
});
