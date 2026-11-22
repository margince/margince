// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../testing/mefixture";
import { renderWithQueryClient } from "../testing/queryrender";
import { steppedClock } from "../testing/steppedclock";
import { type Command, CommandPalette } from "./palette";

// What a screen reader hears of the palette. The search box is a combobox over
// the results, and it names the row the arrows are on.

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

const commands: Command[] = ["Deals", "Deal stages", "Deal settings"].map(
  (label, index) => ({
    id: `screen:${index}`,
    label,
    type: "screen",
    route: { screen: "deals" },
  }),
);

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
    meFixture({ rowScope: "all" }),
  );
}

function activeOption(input: HTMLElement): HTMLElement | null {
  const id = input.getAttribute("aria-activedescendant");
  return id === null ? null : document.getElementById(id);
}

describe("the command palette for a screen reader", () => {
  it("names the row the arrows highlight as the active, selected option", async () => {
    const user = steppedClock();
    hits([]);
    renderPalette();
    const input = screen.getByRole("combobox", { name: "Command palette" });
    await user.type(input, "deal");
    await screen.findByRole("option", { name: /^Deal settings/ });

    expect(input.getAttribute("aria-expanded")).toBe("true");
    expect(input.getAttribute("aria-controls")).toBe(
      screen.getByRole("listbox").id,
    );
    expect(activeOption(input)).toBe(
      screen.getByRole("option", { name: /^Deals/ }),
    );

    await user.keyboard("{ArrowDown}{ArrowDown}");
    const options = screen.getAllByRole("option");
    expect(activeOption(input)).toBe(
      screen.getByRole("option", { name: /^Deal settings/ }),
    );
    expect(options[2]).toBe(activeOption(input));
    expect(
      options.map((option) => option.getAttribute("aria-selected")),
    ).toEqual(options.map((option) => String(option === options[2])));
  });

  it("lists record hits in a group named by their kind", async () => {
    const user = steppedClock();
    hits([{ type: "company", id: "o1", title: "Brandt GmbH" }]);
    renderPalette();
    await user.type(screen.getByRole("combobox"), "brandt");
    const hit = await screen.findByRole("option", { name: /Brandt GmbH/ });
    const group = within(screen.getByRole("listbox")).getByRole("group", {
      name: "Companies",
    });
    expect(group.contains(hit)).toBe(true);
  });

  it("points at no option once nothing is listed", async () => {
    const user = steppedClock();
    hits([]);
    renderPalette();
    const input = screen.getByRole("combobox");
    await user.type(input, "zzzz");
    await screen.findByText("No matches for “zzzz”.");
    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(input.getAttribute("aria-expanded")).toBe("false");
    expect(input.hasAttribute("aria-activedescendant")).toBe(false);
  });
});
