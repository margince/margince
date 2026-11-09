/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithQueryClient } from "../testing/queryrender";
import { steppedClock } from "../testing/steppedclock";
import { type Command, CommandPalette } from "./palette";

// Where Enter lands when the reader has not picked a row. A screen the words
// match keeps it; palette.test.tsx holds that. A record found by search takes
// it only when the words name it whole; otherwise the results page does. A row
// that runs leaves the palette closed.

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
  renderWithQueryClient(
    <CommandPalette open onClose={() => {}} commands={commands} />,
  );
}

function PaletteWithOpener() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Search
      </button>
      <CommandPalette
        open={open}
        onClose={() => setOpen(false)}
        commands={commands}
      />
    </>
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

  it("stays closed after Enter runs a row from a palette a button opened", async () => {
    const user = steppedClock();
    hits([]);
    renderWithQueryClient(<PaletteWithOpener />);
    const opener = screen.getByRole("button", { name: "Search" });
    await user.click(opener);
    await user.type(screen.getByRole("searchbox"), "deals");
    await user.keyboard("{Enter}");
    expect(document.activeElement).toBe(opener);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(window.location.hash).toBe("#/deals");
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

  // A meeting, call, note or task has no page, and is offered all the same:
  // it opens on the results, in the activity panel.
  it("offers a meeting search found and opens it in the activity panel", async () => {
    const user = steppedClock();
    hits([
      { type: "activity", id: "a2", title: "Quarterly review with Demo GmbH" },
    ]);
    renderPalette();
    await user.type(screen.getByRole("searchbox"), "quarterly");
    await screen.findByRole("button", { name: /Quarterly review/ });
    await user.keyboard("{ArrowDown}{Enter}");
    expect(window.location.hash).toBe("#/search/quarterly/a2/activity");
  });
});

it("shows the address match above its normal group and opens the same lead", async () => {
  const user = steppedClock();
  hits([
    { type: "company", id: "o1", title: "Example" },
    { type: "lead", id: "l1", title: "Lena", identifier_match: "prefix" },
  ]);
  renderPalette();
  await user.type(screen.getByRole("searchbox"), "lena@exam");
  await screen.findByText("Best match");
  const rows = screen.getAllByRole("button", { name: /Lena/ });
  expect(rows).toHaveLength(2);
  await user.click(rows[0]);
  expect(window.location.hash).toBe("#/leads/l1");
});
