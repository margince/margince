// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The strip that narrows the palette to one type of record. It counts what the
// whole answer found. A pick asks the server for more of that type alone.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SEARCH_DEBOUNCE_MS } from "../design-system/debouncedsearch";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { steppedClock } from "../testing/steppedclock";
import { type Command, CommandPalette } from "./palette";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

const COMMANDS: Command[] = [
  {
    id: "screen:deals",
    label: "Deals",
    keywords: ["acme"],
    type: "screen",
    route: { screen: "deals" },
  },
];

const EVERY_TYPE = {
  data: [
    { type: "contact", id: "p1", title: "Dana Acme" },
    { type: "deal", id: "d1", title: "Acme renewal" },
    { type: "deal", id: "d2", title: "Acme pilot" },
  ],
  page: { next_cursor: null, has_more: false },
  types_with_more: ["deal"],
};

const DEALS_ONLY = {
  data: [
    { type: "deal", id: "d1", title: "Acme renewal" },
    { type: "deal", id: "d2", title: "Acme pilot" },
    { type: "deal", id: "d3", title: "Acme fleet" },
  ],
  page: { next_cursor: null, has_more: false },
  types_with_more: [],
};

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function searchParams(input: RequestInfo | URL): URLSearchParams {
  const url = input instanceof Request ? input.url : String(input);
  return new URL(url, "http://localhost").searchParams;
}

function renderPalette() {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) =>
    json(searchParams(input).get("types") ? DEALS_ONLY : EVERY_TYPE),
  );
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryDefaults(["me"], { staleTime: Infinity });
  client.setQueryData(["me"], meFixture({}));
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CommandPalette open onClose={() => {}} commands={COMMANDS} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return fetchMock;
}

async function typeAndSettle(words: string) {
  const user = steppedClock();
  await user.type(screen.getByRole("searchbox"), words);
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
  await screen.findByRole("button", { name: /Acme renewal/ });
  return user;
}

function strip() {
  return within(screen.getByRole("group", { name: "Show only" }));
}

describe("the palette's type strip", () => {
  it("is absent until the words find a record", () => {
    renderPalette();
    expect(screen.queryByRole("group", { name: "Show only" })).toBeNull();
  });

  it("offers each type found, with its count and a floor where more matched", async () => {
    renderPalette();
    await typeAndSettle("acme");
    expect(
      strip()
        .getAllByRole("button")
        .map((option) => option.textContent),
    ).toEqual(["All, 3+", "Contacts, 1", "Deals, 2+"]);
  });

  it("asks as a typeahead and keeps a trailing blank that finishes the word", async () => {
    const fetchMock = renderPalette();
    await typeAndSettle("acme ");
    const asked = searchParams(fetchMock.mock.calls.at(-1)?.[0] ?? "");
    expect(asked.get("q")).toBe("acme ");
    expect(asked.get("typeahead")).toBe("true");
  });

  it("narrows the list to the type picked and asks for more of it", async () => {
    const fetchMock = renderPalette();
    const user = await typeAndSettle("acme");
    await user.click(strip().getByRole("button", { name: /Deals/ }));
    await screen.findByRole("button", { name: /Acme fleet/ });

    const narrowed = searchParams(fetchMock.mock.calls.at(-1)?.[0] ?? "");
    expect(narrowed.get("types")).toBe("deal");
    expect(narrowed.get("per_type")).toBe("8");
    expect(screen.queryByRole("button", { name: /Dana Acme/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Deals Screen/ })).toBeNull();
    expect(
      strip()
        .getByRole("button", { name: /Deals/ })
        .getAttribute("aria-pressed"),
    ).toBe("true");
  });
});
