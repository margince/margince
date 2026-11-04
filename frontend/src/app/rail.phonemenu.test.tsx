// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { act, cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { navigate } from "../lib/router";
import { replaceParams } from "../lib/urlstate";
import {
  ignoreSearch,
  render,
  stubPhoneViewport,
} from "../testing/shellharness";
import { Shell } from "./shell";

// The phone bar's More menu in the whole shell. What closes it, where focus goes
// then, and what it locks while open.

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

const more = () => screen.getByRole("button", { name: "More" });
const menu = () => document.querySelector(".railmenu");

function renderPhoneShell(hash: string) {
  const viewport = stubPhoneViewport();
  window.location.hash = hash;
  render(<Shell onOpenSearch={ignoreSearch}>{null}</Shell>);
  return viewport;
}

describe("the phone More menu", () => {
  // Back then Forward lands on the address the menu opened at. The menu, its
  // scrim and the locked page must not come back with it.
  it("stays closed when the reader leaves its address and comes back", async () => {
    const user = userEvent.setup();
    renderPhoneShell("#/home");
    await user.click(more());
    expect(menu()).not.toBeNull();

    navigate({ screen: "deals" });
    await waitFor(() => expect(menu()).toBeNull());
    navigate({ screen: "home" });
    await waitFor(() => expect(window.location.hash).toBe("#/home"));

    expect(menu()).toBeNull();
    expect(document.querySelector(".railscrim")).toBeNull();
    expect(more().getAttribute("aria-expanded")).toBe("false");
    expect(document.querySelector<HTMLElement>(".main")?.inert).toBe(false);
  });

  it("closes when only the address's query changes", async () => {
    const user = userEvent.setup();
    renderPhoneShell("#/deals?stage=open");
    await user.click(more());
    expect(menu()).not.toBeNull();

    act(() => navigate({ screen: "deals" }, new Map([["stage", "won"]])));
    await waitFor(() => expect(menu()).toBeNull());
    expect(document.querySelector<HTMLElement>(".main")?.inert).toBe(false);
  });

  // Two entries that differ only in their query share a path. Their history
  // depth is what tells Back and Forward apart from a dial written in place.
  it("closes on Back and on Forward between two entries of one path", async () => {
    const user = userEvent.setup();
    renderPhoneShell("#/deals?stage=open");
    act(() => navigate({ screen: "deals" }, new Map([["stage", "won"]])));
    await waitFor(() => expect(window.location.hash).toBe("#/deals?stage=won"));
    const main = () => document.querySelector<HTMLElement>(".main");

    await user.click(more());
    expect(menu()).not.toBeNull();
    act(() => history.back());
    await waitFor(() =>
      expect(window.location.hash).toBe("#/deals?stage=open"),
    );
    await waitFor(() => expect(menu()).toBeNull());
    expect(main()?.inert).toBe(false);

    await user.click(more());
    expect(menu()).not.toBeNull();
    act(() => history.forward());
    await waitFor(() => expect(window.location.hash).toBe("#/deals?stage=won"));
    await waitFor(() => expect(menu()).toBeNull());
    expect(main()?.inert).toBe(false);
  });

  // A dial written in place (a search box settling) keeps the entry and the page.
  it("stays open when a dial is written over the current entry", async () => {
    const user = userEvent.setup();
    renderPhoneShell("#/deals?q=ac");
    await user.click(more());
    expect(menu()).not.toBeNull();

    act(() => replaceParams(new Map([["q", "acme"]])));
    expect(window.location.hash).toBe("#/deals?q=acme");
    expect(menu()).not.toBeNull();
  });

  // The pressed row unmounts with the menu. More now stands for the page it
  // opened, as the pressed row does in the desktop sidebar, so focus waits there.
  it("hands focus to More when a menu row opens a page", async () => {
    const user = userEvent.setup();
    renderPhoneShell("#/home");
    await user.click(more());

    await user.click(screen.getByRole("link", { name: "Companies" }));
    await waitFor(() => expect(window.location.hash).toBe("#/companies"));

    expect(menu()).toBeNull();
    expect(document.activeElement).toBe(more());
    expect(more().getAttribute("aria-current")).toBe("page");
  });

  // Back and Forward close the menu during render, so no row press hands focus on.
  it("hands focus to More when a new address closes the menu", async () => {
    const user = userEvent.setup();
    renderPhoneShell("#/home");
    await user.click(more());
    expect(menu()?.contains(document.activeElement)).toBe(true);

    act(() => navigate({ screen: "deals" }));
    await waitFor(() => expect(menu()).toBeNull());

    expect(document.activeElement).toBe(more());
  });

  // Past the phone width there is no More, so the sidebar's current row takes
  // the focus the menu held.
  it("hands focus to the sidebar's current row when the window widens", async () => {
    const user = userEvent.setup();
    const viewport = renderPhoneShell("#/home");
    await user.click(more());
    expect(menu()?.contains(document.activeElement)).toBe(true);

    act(() => viewport.widen());

    expect(menu()).toBeNull();
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Home");
    expect(document.activeElement?.getAttribute("aria-current")).toBe("page");
  });

  // A section's level leads with Back, which is no destination, so it is passed.
  it.each([
    ["the top level", "#/nowhere"],
    ["a section level", "#/settings/nowhere"],
  ])(
    "hands focus to the first destination of %s on a widen where no row is current",
    async (_level, hash) => {
      const user = userEvent.setup();
      const viewport = renderPhoneShell(hash);
      await user.click(more());
      expect(menu()?.contains(document.activeElement)).toBe(true);

      act(() => viewport.widen());

      expect(menu()).toBeNull();
      expect(document.querySelector(".navitem[aria-current]")).toBeNull();
      expect(document.activeElement).toBe(
        document.querySelector(".navlevel .navwrap .navitem"),
      );
      expect(document.activeElement?.classList.contains("navback")).toBe(false);
    },
  );

  it("locks the page and the skip link while open, and frees them after", async () => {
    const user = userEvent.setup();
    renderPhoneShell("#/home");
    const main = document.querySelector<HTMLElement>(".main");
    const skip = document.querySelector<HTMLElement>(".skiplink");
    if (!main || !skip) {
      throw new Error("the shell drew no page or no skip link");
    }
    expect([main.inert, skip.inert]).toEqual([false, false]);

    await user.click(more());
    expect([main.inert, skip.inert]).toEqual([true, true]);

    await user.keyboard("{Escape}");
    expect(menu()).toBeNull();
    expect([main.inert, skip.inert]).toEqual([false, false]);
  });
});
