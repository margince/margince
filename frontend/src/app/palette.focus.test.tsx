/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { holdExits, watchHiding } from "../design-system/presence-testing";
import { LocaleProvider } from "../i18n";
import { renderWithQueryClient } from "../testing/queryrender";
import { steppedClock } from "../testing/steppedclock";
import { type Command, CommandPalette } from "./palette";

// One screen command is enough: this file asks where focus goes when the
// palette closes, not what it lists.
const commands: Command[] = [
  {
    id: "screen:deals",
    label: "Deals",
    keywords: ["pipeline"],
    type: "screen",
    route: { screen: "deals" },
  },
];

// The exit is held, so the overlay is still mounted, inert and hidden, when
// each case asks where focus went.
let exits: ReturnType<typeof holdExits>;
let hiding: ReturnType<typeof watchHiding>;

beforeEach(() => {
  exits = holdExits();
  hiding = watchHiding();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [],
            page: { next_cursor: null, has_more: false },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
});

afterEach(() => {
  cleanup();
  hiding.restore();
  exits.mockRestore();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function PaletteFromButton() {
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

function overlay(): Element {
  const found = document.querySelector(".palette-overlay");
  if (!found) throw new Error("the palette is not on the page");
  return found;
}

// Focus is out of the overlay before it leaves the accessibility tree, and the
// overlay did leave it: a check that saw no hiding at all would pass vacuously.
function expectHiddenAfterFocusLeft() {
  expect(hiding.hidings.filter((write) => write.heldFocus)).toEqual([]);
  expect(hiding.hidings.map((write) => write.element)).toContain(overlay());
  expect(overlay().hasAttribute("inert")).toBe(true);
}

describe("the palette hands focus back before it hides its overlay", () => {
  async function openFromButton() {
    const user = steppedClock();
    renderWithQueryClient(<PaletteFromButton />);
    const opener = screen.getByRole("button", { name: "Search" });
    await user.click(opener);
    expect(document.activeElement).toBe(screen.getByRole("searchbox"));
    return { user, opener };
  }

  it("on Escape", async () => {
    const { user, opener } = await openFromButton();
    await user.keyboard("{Escape}");
    expectHiddenAfterFocusLeft();
    expect(document.activeElement).toBe(opener);
  });

  it("on a press on the backdrop", async () => {
    const { opener } = await openFromButton();
    fireEvent.pointerDown(overlay());
    fireEvent.pointerUp(overlay());
    fireEvent.click(overlay());
    expectHiddenAfterFocusLeft();
    expect(document.activeElement).toBe(opener);
  });

  it("on Enter, running the selected row", async () => {
    const { user, opener } = await openFromButton();
    await user.type(screen.getByRole("searchbox"), "deals");
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/deals");
    expectHiddenAfterFocusLeft();
    expect(document.activeElement).toBe(opener);
  });

  it("when the page behind closes it", () => {
    const client = new QueryClient();
    const page = (open: boolean) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <button type="button">Search</button>
          <CommandPalette open={open} onClose={() => {}} commands={commands} />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const { rerender } = render(page(false));
    const opener = screen.getByRole("button", { name: "Search" });
    opener.focus();
    rerender(page(true));
    expect(document.activeElement).toBe(screen.getByRole("searchbox"));

    rerender(page(false));

    expectHiddenAfterFocusLeft();
    expect(document.activeElement).toBe(opener);
  });
});
