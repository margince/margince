// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { echoedQuery, SearchRoute, SearchScreen } from "./search";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({ rowScope: "all" }));
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

/** Answers every search with these hits, and records what was asked. */
function serve(hits: readonly unknown[]): string[] {
  const asked: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      asked.push(url);
      return Response.json({
        data: hits,
        page: { next_cursor: null, has_more: false },
      });
    }),
  );
  return asked;
}

const deal = {
  type: "deal",
  id: "00000000-0000-4000-8000-0000000000d1",
  title: "Fleet retrofit",
  standing: {
    state: "open",
    stage: "Proposal",
    amount_minor: 1_200_000,
    currency: "EUR",
  },
};

const task = {
  type: "activity",
  id: "00000000-0000-4000-8000-0000000000a1",
  title: "Follow up with the new lead",
  snippet: "Ask about the second site",
};

// Deals and activities are records too: each stands in the same box as a
// contact, with what the hit carries to tell look-alikes apart.
describe("deal and activity hits", () => {
  it("draws a deal in the record box with its stage and amount", async () => {
    serve([deal]);
    render(<SearchScreen q="fleet" />);
    const link = await screen.findByRole("link", { name: /Fleet retrofit/ });
    const card = link.closest(".record-card");
    if (!(card instanceof HTMLElement)) throw new Error("no record box");
    expect(within(card).getByText("Proposal")).toBeTruthy();
    expect(card.textContent).toMatch(/12,000/);
  });

  it("draws an activity in the record box, opening its panel", async () => {
    serve([task]);
    render(<SearchScreen q="follow" />);
    const open = await screen.findByRole("button", {
      name: /Follow up with the new lead/,
    });
    expect(open.closest(".record-card")).not.toBeNull();
    expect(open.getAttribute("aria-haspopup")).toBe("dialog");
    expect(screen.getByText("“Ask about the second site”")).toBeTruthy();
  });
});

describe("the search address", () => {
  it("reads the term from ?q= when the path carries none", async () => {
    const asked = serve([]);
    window.location.hash = "#/search?q=alice";
    render(<SearchRoute q="" />);
    expect(await screen.findByText("No matches for “alice”.")).toBeTruthy();
    expect(asked.some((url) => url.includes("q=alice"))).toBe(true);
  });

  it("repeats a long term cut short, so the sentence stays a sentence", async () => {
    serve([]);
    const long = "x".repeat(400);
    render(<SearchScreen q={long} />);
    const said = await screen.findByText(/^No matches for/);
    expect(said.textContent).toBe(`No matches for “${"x".repeat(80)}…”.`);
  });
});

describe("echoedQuery", () => {
  it("keeps a short term whole and never splits a character", () => {
    expect(echoedQuery("acme")).toBe("acme");
    expect(echoedQuery("ü".repeat(81))).toBe(`${"ü".repeat(80)}…`);
  });
});
