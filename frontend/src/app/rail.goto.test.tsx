// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { goToBindingFor } from "../lib/keymap";
import { routeHash } from "../lib/router";
import {
  setSingleKeyShortcutsOn,
  useShortcutDispatcher,
} from "../lib/shortcuts";
import { render } from "../testing/shellharness";
import { NAV } from "./nav";
import { WorkspaceRail } from "./shell";

afterEach(() => {
  cleanup();
  act(() => setSingleKeyShortcutsOn(true));
  window.location.hash = "";
});

describe("the destinations' go-to keys", () => {
  // The rail and the dialog name a destination with one word.
  it("bind every destination, in the rail's own words", () => {
    for (const item of NAV) {
      const bound = goToBindingFor(item.screen);
      expect(bound?.labelKey, item.screen).toBe(item.labelKey);
    }
  });

  it("are announced on each row", () => {
    render(<WorkspaceRail route={{ screen: "home" }} />);
    expect(screen.getByRole("link", { name: "Contacts" })).toHaveAttribute(
      "aria-keyshortcuts",
      "g c",
    );
  });

  it("show on an expanded row's tip, which the label already names", async () => {
    render(<WorkspaceRail route={{ screen: "home" }} />);
    act(() => screen.getByRole("link", { name: "Leads" }).focus());
    expect((await screen.findByRole("tooltip")).textContent).toBe("G then L");
  });

  it("take the reader there on G, then the letter", async () => {
    const user = userEvent.setup();
    renderHook(() => useShortcutDispatcher());
    render(<WorkspaceRail route={{ screen: "home" }} />);

    await user.keyboard("gp");
    expect(window.location.hash).toBe(routeHash({ screen: "projects" }));
  });

  it("leave the rail unmarked and untipped while single keys are off", async () => {
    act(() => setSingleKeyShortcutsOn(false));
    render(<WorkspaceRail route={{ screen: "home" }} collapsed />);
    const deals = screen.getByRole("link", { name: "Deals" });
    expect(deals).not.toHaveAttribute("aria-keyshortcuts");
    act(() => deals.focus());
    expect((await screen.findByRole("tooltip")).textContent).toBe(
      en["nav.deals"],
    );
  });

  it("raise no tip on an expanded row while single keys are off", () => {
    act(() => setSingleKeyShortcutsOn(false));
    render(<WorkspaceRail route={{ screen: "home" }} />);
    act(() => screen.getByRole("link", { name: "Deals" }).focus());
    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});
