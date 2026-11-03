// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { navigate } from "../lib/router";
import {
  ignoreSearch,
  render,
  stubPhoneViewport,
} from "../testing/shellharness";
import type { railTrail } from "./nav";
import { Shell, WorkspaceRail } from "./shell";

// The phone bar's More menu in the whole shell. What closes it, where focus goes
// then, what it locks while open, and when it is not drawn.

// A seat that sees only the bar's destinations, made by narrowing the trail the
// rail reads to its bar rows. Off by default, so every other case reads the
// real trail.
const seat = vi.hoisted(() => ({ barOnly: false }));

vi.mock("./nav", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./nav")>();
  const trail: typeof railTrail = (route, section) =>
    actual.railTrail(route, section).map((level) =>
      seat.barOnly
        ? {
            ...level,
            groups: level.groups.map((group) => ({
              ...group,
              items: group.items.filter((entry) => level.barIds?.has(entry.id)),
            })),
          }
        : level,
    );
  return { ...actual, railTrail: trail };
});

afterEach(() => {
  cleanup();
  seat.barOnly = false;
  window.location.hash = "";
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

const more = () => screen.getByRole("button", { name: "More" });
const menu = () => document.querySelector(".railmenu");

function renderPhoneShell(hash: string) {
  stubPhoneViewport();
  window.location.hash = hash;
  return render(<Shell onOpenSearch={ignoreSearch}>{null}</Shell>);
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

  // The desktop sidebar moves no focus after a row press, and neither does this
  // menu. The row unmounts, and More leaves the new page's focus alone.
  it("leaves focus to the page a menu row opens", async () => {
    const user = userEvent.setup();
    renderPhoneShell("#/home");
    await user.click(more());

    await user.click(screen.getByRole("link", { name: "Companies" }));
    await waitFor(() => expect(window.location.hash).toBe("#/companies"));

    expect(menu()).toBeNull();
    expect(document.activeElement).not.toBe(more());
    expect(document.activeElement).toBe(document.body);
  });

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

  it("draws no More and no menu for a seat whose destinations fit the bar", () => {
    seat.barOnly = true;
    stubPhoneViewport();
    const { container } = render(<WorkspaceRail route={{ screen: "home" }} />);

    expect(
      [...container.querySelectorAll(".rail > .navlevel .navitem")].map((row) =>
        row.getAttribute("aria-label"),
      ),
    ).toEqual(["Home", "Contacts", "Deals"]);
    expect(container.querySelector(".railmore")).toBeNull();
    expect(screen.queryByRole("button", { name: "More" })).toBeNull();
    expect(menu()).toBeNull();
  });
});
