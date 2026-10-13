/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { steppedClock } from "../testing/steppedclock";
import { type Command, CommandPalette } from "./palette";

// Where Enter lands when the reader has not picked a row. A screen the words
// match keeps it (palette.test.tsx holds that); a record found by search takes
// it only when the words name it whole, and the results page otherwise.

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

const commands: Command[] = [
  {
    id: "screen:deals",
    label: "Deals",
    keywords: ["pipeline"],
    type: "screen",
    route: { screen: "deals" },
  },
];

function hits(data: readonly Record<string, string>[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data,
            page: { next_cursor: null, has_more: false },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
}

function renderPalette() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CommandPalette open onClose={() => {}} commands={commands} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("CommandPalette Enter", () => {
  // Half a name is still a search: a reader who stops typing at "strai" wants
  // to see what matched, so Enter opens the results rather than one record.
  it("opens every result on Enter when the words only begin a record's name", async () => {
    const user = steppedClock();
    hits([{ type: "company", id: "o1", title: "Straight" }]);
    renderPalette();
    await user.type(screen.getByRole("searchbox"), "strai");
    await screen.findByRole("button", { name: /Straight/ });
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/search/strai");
  });

  it("opens the record on Enter when the words name it whole", async () => {
    const user = steppedClock();
    hits([
      { type: "company", id: "o1", title: "Straight" },
      { type: "contact", id: "p1", title: "Anna Becker" },
    ]);
    renderPalette();
    await user.type(screen.getByRole("searchbox"), "straight");
    await screen.findByRole("button", { name: /Straight/ });
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/companies/o1");
  });

  // Until the search answers, the palette cannot know the words name a record,
  // and the results page is where a reader lands; the record leads it there.
  it("opens every result on Enter pressed before the search has answered", async () => {
    const user = steppedClock();
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    renderPalette();
    await user.type(screen.getByRole("searchbox"), "straight");
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/search/straight");
  });

  // The arrows still choose: the see-all row leads, and the hits sit one
  // press below it.
  it("opens the hit the reader arrowed to", async () => {
    const user = steppedClock();
    hits([{ type: "company", id: "o1", title: "Straight" }]);
    renderPalette();
    await user.type(screen.getByRole("searchbox"), "strai");
    await screen.findByRole("button", { name: /Straight/ });
    await user.keyboard("{ArrowDown}{Enter}");
    expect(window.location.hash).toBe("#/companies/o1");
  });

  it("goes to the screen the words name rather than to the results", async () => {
    const user = steppedClock();
    hits([{ type: "deal", id: "d1", title: "Pipeline review" }]);
    renderPalette();
    await user.type(screen.getByRole("searchbox"), "pipeline");
    await screen.findByRole("button", { name: /Pipeline review/ });
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/deals");
  });
});
