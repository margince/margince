/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { PersonNetworkCard } from "./network";

// The relationship-graph card. What is worth pinning is not that it renders a
// list — it is the two readings that would quietly mislead a rep: an unspoken
// relationship shown as a zero, and a server ranking a client re-sorted.

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

function stubRoutes(routes: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const key = url.pathname.replace(/^\/v1/, "");
      if (key in routes) {
        return jsonResponse(routes[key]);
      }
      return jsonResponse({ title: "not found" }, 404);
    }),
  );
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("PersonNetworkCard", () => {
  it("keeps the server's warmest-first order instead of re-ranking", async () => {
    // The score is computed at read from a decay formula. A client that sorted
    // these itself would be a second implementation of that formula, and the
    // two would disagree the moment either changed. The fixture is deliberately
    // NOT in interaction-count order, so a client-side sort would show.
    stubRoutes({
      "/people/p-1/network": {
        person_id: "p-1",
        colleagues: [
          {
            user_id: "u-1",
            display_name: "Anna Weber",
            strength: 81,
            strength_bucket: "strong",
            interactions_90d: 6,
            last_at: "2026-07-20T10:00:00Z",
          },
          {
            user_id: "u-2",
            display_name: "Jonas Bach",
            strength: 30,
            strength_bucket: "weak",
            interactions_90d: 40,
            last_at: "2026-07-29T10:00:00Z",
          },
        ],
      },
    });
    render(<PersonNetworkCard id="p-1" />);
    const rows = await screen.findAllByRole("listitem");
    expect(within(rows[0]).getByText("Anna Weber")).toBeInTheDocument();
    expect(within(rows[1]).getByText("Jonas Bach")).toBeInTheDocument();
  });

  it("shows no score for a colleague who has never spoken to them", async () => {
    // A `none` band carries no number. Rendering a 0 makes "we have never
    // spoken" look identical to "we spoke and it went cold", and a rep picks
    // their route in from exactly that difference.
    stubRoutes({
      "/people/p-1/network": {
        person_id: "p-1",
        colleagues: [
          {
            user_id: "u-1",
            display_name: "Anna Weber",
            strength: null,
            strength_bucket: "none",
            interactions_90d: 0,
            last_at: null,
          },
        ],
      },
    });
    render(<PersonNetworkCard id="p-1" />);
    expect(await screen.findByText("Anna Weber")).toBeInTheDocument();
    expect(screen.getByText(/no contact/i)).toBeInTheDocument();
    expect(screen.getByText(/no recorded contact/i)).toBeInTheDocument();
  });

  it("says nobody knows them rather than rendering an empty card", async () => {
    stubRoutes({
      "/people/p-1/network": { person_id: "p-1", colleagues: [] },
    });
    render(<PersonNetworkCard id="p-1" />);
    expect(
      await screen.findByText(/nobody here has recorded contact/i),
    ).toBeInTheDocument();
  });
});
