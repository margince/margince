// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../i18n/en";
import type { Route } from "../lib/router";
import {
  ignoreSearch,
  newClient,
  render,
  renderWith,
  repSeeing,
  stubPhoneViewport,
  TWO_MARKS,
} from "../testing/shellharness";
import { TopBar } from "./topbar";

// The strip above every railed page: the search, then what is waiting and who
// is signed in. The account menu and the bell have suites of their own.

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

function renderTopBar(
  route: Route,
  extra: Readonly<{ onOpenSearch?: () => void }> = {},
) {
  return render(
    <TopBar route={route} onOpenSearch={extra.onOpenSearch ?? ignoreSearch} />,
  );
}

// AC-shell-7: ONE search affordance in the product, and it is this one. It is a
// button, not a field — it opens the palette and never accepts inline typing.
describe("Top bar search (AC-shell-7)", () => {
  it("opens the palette and never takes the query itself", async () => {
    const user = userEvent.setup();
    const onOpenSearch = vi.fn();
    const { container } = renderTopBar({ screen: "home" }, { onOpenSearch });

    await user.click(
      screen.getByRole("button", {
        name: englishCopy["shell.searchEverything"],
      }),
    );
    expect(onOpenSearch).toHaveBeenCalledTimes(1);
    // A field here would be a second search that answers to nothing: the
    // palette owns the query, the bar only opens it.
    expect(within(container).queryByRole("textbox")).toBeNull();
  });

  it("is a button rather than the field it is styled as", () => {
    const { container } = renderTopBar({ screen: "home" });
    expect(container.querySelector(".topbar-search")?.tagName).toBe("BUTTON");
  });

  // The shortcut cap is a hint about how else to get here, not a second name: a
  // reader who says "Search or ask Margince" must reach it, and none of them should
  // be made to spell out ⌘K. `name` is an exact match on the computed name, so
  // a kbd that leaked into it fails this.
  it("is named for what it does, with the shortcut kept out of that name", () => {
    const { container } = renderTopBar({ screen: "home" });
    expect(
      screen.getByRole("button", {
        name: englishCopy["shell.searchEverything"],
      }),
    ).toBeTruthy();
    // One cap per key, and the GROUP is what is hidden — a per-cap attribute
    // would leave the group announcing itself as the caps' container.
    const keys = container.querySelector(".topbar-keys");
    expect(keys?.getAttribute("aria-hidden")).toBe("true");
    // Two caps, whichever platform the test host claims to be: the chord reads
    // "⌘ then K" on a Mac and "Ctrl then K" elsewhere, and the split is what
    // keeps those one source rather than two spellings.
    // `kbd.kbd`: the caps in this strip ARE the design system's one key cap
    // (`Kbd`), not a box the top bar draws for itself, so the selector fails if
    // the strip goes back to spelling its own.
    const caps = [...(keys?.querySelectorAll("kbd.kbd") ?? [])].map(
      (cap) => cap.textContent,
    );
    expect(caps).toHaveLength(2);
    expect(caps[1]).toBe("K");
    expect(caps[0]).toMatch(/^(⌘|Ctrl)$/);
  });

  // The name is the printed words alone, so a speech-input reader's "click
  // Search or ask Margince" matches it; a screen reader hears the chord apart.
  it("takes its name from its printed words and announces the chord apart", () => {
    renderTopBar({ screen: "home" });
    const search = screen.getByRole("button", {
      name: englishCopy["shell.searchEverything"],
    });
    expect(search.hasAttribute("aria-label")).toBe(false);
    // Both modifiers answer, so both are announced: the keymap's own value.
    expect(search.getAttribute("aria-keyshortcuts")).toBe("Meta+K Control+K");
  });
});

// The page heading in the card names the page, so the bar carries no trail.
describe("Top bar's lead", () => {
  it("leads home from the company's mark at phone width", () => {
    stubPhoneViewport();
    const client = newClient();
    client.setQueryData(["me"], repSeeing(TWO_MARKS));
    renderWith(
      client,
      <TopBar route={{ screen: "deals" }} onOpenSearch={ignoreSearch} />,
    );
    // The name starts with what the chip prints (WCAG 2.5.3): the initials until
    // the square mark paints, then nothing.
    const home = screen.getByRole("link", {
      name: "DG Demo GmbH home, powered by Margince",
    });
    expect(home.getAttribute("href")).toBe("/#/home");
    const mark = home.querySelector(".ws-chip-company img");
    expect(mark?.getAttribute("src")).toBe(TWO_MARKS.logo_icon_url);
    if (mark) fireEvent.load(mark);
    expect(
      screen.getByRole("link", { name: "Demo GmbH home, powered by Margince" }),
    ).toBe(home);
  });

  it("draws no trail and no mark on a wide screen", () => {
    const { container } = renderTopBar({ screen: "deals" });
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(container.querySelector(".topbar-lead")).toBeNull();
    expect(container.querySelector(".topbar .ws-chip")).toBeNull();
  });

  it("leads a record with its back control alone", () => {
    const { container } = renderTopBar({ screen: "contacts", id: "p-anna" });
    const lead = container.querySelector<HTMLElement>(".topbar-lead");
    expect(lead).not.toBeNull();
    const inLead = within(lead ?? container);
    expect(inLead.getAllByRole("button")).toHaveLength(1);
    expect(inLead.getByRole("button", { name: "Back" })).toBeTruthy();
    expect(screen.queryByRole("navigation")).toBeNull();
  });
});
