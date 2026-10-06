/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ListPopover, type ListPopoverOption } from "./listpopover";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const OWNERS: readonly ListPopoverOption[] = [
  { id: "u-1", name: "Anna Weber" },
  { id: "u-2", name: "Otto Fischer" },
  { id: "u-3", name: "Lena Brandt" },
];

function activeName(): string | null {
  const id = screen.getByRole("combobox").getAttribute("aria-activedescendant");
  return id ? (document.getElementById(id)?.textContent ?? null) : null;
}

function stubPhone() {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: /max-width:\s*700px/.test(query),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

describe("ListPopover on a desktop", () => {
  it("opens a non-modal dialog named by its trigger, with focus in the search", async () => {
    const user = userEvent.setup();
    render(
      <ListPopover
        label="Assign owner"
        title="Colleagues"
        searchLabel="Search colleagues"
        options={OWNERS}
        onPick={vi.fn()}
      />,
    );
    const trigger = screen.getByRole("button", { name: "Assign owner" });
    expect(trigger.getAttribute("aria-haspopup")).toBe("dialog");
    expect(trigger.getAttribute("aria-expanded")).toBe("false");

    await user.click(trigger);

    const panel = screen.getByRole("dialog", { name: "Assign owner" });
    expect(panel.getAttribute("aria-modal")).toBeNull();
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(
      screen.getByRole("combobox", { name: "Search colleagues" }),
    );
    expect(
      screen.getAllByRole("option").map((option) => option.textContent),
    ).toEqual(["Anna Weber", "Otto Fischer", "Lena Brandt"]);
  });

  it("walks the list with the arrows, Home and End, and filters as the reader types", async () => {
    const user = userEvent.setup();
    render(
      <ListPopover
        label="Assign owner"
        title="Colleagues"
        searchLabel="Search colleagues"
        options={OWNERS}
        onPick={vi.fn()}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Assign owner" }));

    await user.keyboard("{ArrowDown}");
    expect(activeName()).toBe("Anna Weber");
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}");
    expect(activeName()).toBe("Lena Brandt");
    await user.keyboard("{Home}");
    expect(activeName()).toBe("Anna Weber");
    await user.keyboard("{End}");
    expect(activeName()).toBe("Lena Brandt");

    await user.type(screen.getByRole("combobox"), "fisch");
    expect(activeName()).toBeNull();
    expect(
      screen.getAllByRole("option").map((option) => option.textContent),
    ).toEqual(["Otto Fischer"]);
  });

  it("picks the active row on Enter, closes on done and hands focus back", async () => {
    const user = userEvent.setup();
    const onPick = vi.fn((_: ListPopoverOption, done: () => void) => done());
    render(
      <ListPopover
        label="Assign owner"
        title="Colleagues"
        searchLabel="Search colleagues"
        options={OWNERS}
        onPick={onPick}
      />,
    );
    const trigger = screen.getByRole("button", { name: "Assign owner" });
    await user.click(trigger);

    await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");

    expect(onPick).toHaveBeenCalledWith(OWNERS[1], expect.any(Function));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("keeps the panel open over a refused write until the caller calls done", async () => {
    const user = userEvent.setup();
    const onPick = vi.fn();
    const { rerender } = render(
      <ListPopover
        label="Assign owner"
        title="Colleagues"
        searchLabel="Search colleagues"
        options={OWNERS}
        onPick={onPick}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Assign owner" }));
    await user.click(screen.getByRole("option", { name: "Anna Weber" }));
    rerender(
      <ListPopover
        label="Assign owner"
        title="Colleagues"
        searchLabel="Search colleagues"
        options={OWNERS}
        onPick={onPick}
        pending
      />,
    );

    await user.click(screen.getByRole("option", { name: "Otto Fischer" }));
    expect(onPick).toHaveBeenCalledTimes(1);

    rerender(
      <ListPopover
        label="Assign owner"
        title="Colleagues"
        searchLabel="Search colleagues"
        options={OWNERS}
        onPick={onPick}
        error="Someone else changed this project."
      />,
    );
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByText("Someone else changed this project.")).toBeTruthy();
  });

  it("lists a disabled row without letting it be picked, and marks the selected one", async () => {
    const user = userEvent.setup();
    const onPick = vi.fn();
    render(
      <ListPopover
        label="Add tag"
        title="Tags"
        searchLabel="Search tags"
        options={[
          { id: "t-1", name: "Renewal", hint: "Already added", disabled: true },
          { id: "t-2", name: "Priority" },
        ]}
        selected="t-2"
        onPick={onPick}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Add tag" }));
    const taken = screen.getByRole("option", { name: /Renewal/ });

    await user.click(taken);
    await user.keyboard("{Home}{Enter}");

    expect(onPick).not.toHaveBeenCalled();
    expect(taken.getAttribute("aria-disabled")).toBe("true");
    expect(
      screen
        .getByRole("option", { name: "Priority" })
        .getAttribute("aria-selected"),
    ).toBe("true");
  });

  it("closes on Escape and on a press outside, handing focus back on Escape", async () => {
    const user = userEvent.setup();
    render(
      <>
        <ListPopover
          label="Assign owner"
          title="Colleagues"
          searchLabel="Search colleagues"
          options={OWNERS}
          onPick={vi.fn()}
        />
        <p>Elsewhere on the page</p>
      </>,
    );
    const trigger = screen.getByRole("button", { name: "Assign owner" });
    await user.click(trigger);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(trigger);

    await user.click(trigger);
    await user.click(screen.getByText("Elsewhere on the page"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("says an empty vocabulary is empty in the caller's words", async () => {
    const user = userEvent.setup();
    render(
      <ListPopover
        label="Add tag"
        title="Tags"
        searchLabel="Search tags"
        options={[]}
        empty="Ask an admin to add one."
        onPick={vi.fn()}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Add tag" }));
    expect(screen.getByText("Ask an admin to add one.")).toBeTruthy();
  });

  it("follows a caller that holds the open state", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    const { rerender } = render(
      <ListPopover
        label="Attach project"
        title="Projects"
        searchLabel="Search projects"
        options={OWNERS}
        onPick={vi.fn()}
        open={false}
        onOpenChange={onOpenChange}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Attach project" }));
    expect(onOpenChange).toHaveBeenLastCalledWith(true);
    expect(screen.queryByRole("dialog")).toBeNull();

    rerender(
      <ListPopover
        label="Attach project"
        title="Projects"
        searchLabel="Search projects"
        options={OWNERS}
        onPick={vi.fn()}
        open
        onOpenChange={onOpenChange}
      />,
    );
    expect(screen.getByRole("dialog")).toBeTruthy();
  });
});

describe("ListPopover over a server search", () => {
  // Opened on the real clock, then typed into on a fake one: the debounce is
  // the only timer under test, and one change event is one settled term.
  async function openSearch(
    search: (term: string) => Promise<ListPopoverOption[]>,
  ) {
    render(
      <ListPopover
        label="Attach project"
        title="Projects"
        searchLabel="Search projects"
        search={search}
        onPick={vi.fn()}
      />,
    );
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Attach project" }));
    vi.useFakeTimers();
    return (term: string) =>
      fireEvent.change(screen.getByRole("combobox"), {
        target: { value: term },
      });
  }

  it("asks once the term settles, saying it is searching until the answer lands", async () => {
    const search = vi.fn().mockResolvedValue([{ id: "p-1", name: "Atlas" }]);
    const type = await openSearch(search);

    type("atl");
    expect(screen.getByText("Searching…")).toBeTruthy();
    act(() => {
      vi.advanceTimersByTime(249);
    });
    expect(search).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(search).toHaveBeenCalledTimes(1);
    expect(search).toHaveBeenCalledWith("atl");
    expect(screen.getByRole("option", { name: "Atlas" })).toBeTruthy();
    expect(screen.queryByText("Searching…")).toBeNull();
  });

  it("tells an empty answer from a refused search", async () => {
    const search = vi
      .fn()
      .mockResolvedValueOnce([])
      .mockRejectedValueOnce(new Error("offline"));
    const type = await openSearch(search);

    type("zz");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(screen.getByText("No match")).toBeTruthy();

    type("zzz");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(screen.queryByText("No match")).toBeNull();
    expect(screen.getByRole("alert")).toBeTruthy();
  });
});

describe("ListPopover on a phone", () => {
  it("opens the same search in a sheet headed by its title, and closes it on done", async () => {
    stubPhone();
    const user = userEvent.setup();
    render(
      <ListPopover
        label="Assign owner"
        title="Assign project owner"
        searchLabel="Search colleagues"
        options={OWNERS}
        onPick={(_, done) => done()}
      />,
    );
    const trigger = screen.getByRole("button", { name: "Assign owner" });
    expect(trigger.getAttribute("aria-haspopup")).toBe("dialog");

    await user.click(trigger);
    const sheet = screen.getByRole("dialog", { name: "Assign project owner" });
    expect(sheet.getAttribute("aria-modal")).toBe("true");
    expect(document.activeElement).toBe(screen.getByRole("combobox"));

    await user.keyboard("{End}{Enter}");
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
